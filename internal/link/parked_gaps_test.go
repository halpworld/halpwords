package link

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/halpworld/halpwords/pkg/words"
)

// parkedAge parks one answer for the linked learner, closes, and moves
// the clock on by age.
func parkedAge(t *testing.T, age time.Duration) (*fake, *memStore, *clock) {
	t.Helper()
	f, c, st, clk := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	loseLink(t, f, c)
	c.Close()
	clk.add(age)
	return f, st, clk
}

// Kept events are dropped only when older than parkedFor: exactly
// parkedFor is still kept, a moment more is not.
func TestParkedExpiryBoundary(t *testing.T) {
	f, st, clk := parkedAge(t, parkedFor)
	c2 := reopen(f, st, clk)
	if !st.has(parkedFile) {
		t.Fatal("events dropped at exactly parkedFor")
	}
	relink(t, f, c2)
	if n := len(f.eventsOf("answers")); n != 1 {
		t.Errorf("%d answers sent at exactly parkedFor, want 1", n)
	}
	f, st, clk = parkedAge(t, parkedFor+time.Nanosecond)
	reopen(f, st, clk)
	if st.has(parkedFile) {
		t.Error("events kept one nanosecond past parkedFor")
	}
}

// A damaged link-parked.json must not stop the game opening or syncing,
// and the next loss writes a good file.
func TestDamagedParkedFile(t *testing.T) {
	for name, data := range map[string]string{
		"cut off":    `[{"Learner":"lrn_1","Queue":`,
		"not a list": `{"Learner":"lrn_1"}`,
		"bad time":   `[{"Learner":"lrn_1","At":"yesterday","Queue":{}}]`,
		"empty":      ``,
		"null":       `null`,
	} {
		t.Run(name, func(t *testing.T) {
			f, c, st, clk := linked(t)
			c.Close()
			st.files[parkedFile] = []byte(data)
			c2 := reopen(f, st, clk)
			if err := c2.Sync(context.Background()); err != nil {
				t.Fatal(err)
			}
			c2.Answer("fr", cat, "practice", words.Answer{Tier: words.Perfect})
			loseLink(t, f, c2)
			c3 := reopen(f, st, clk)
			relink(t, f, c3)
			if n := len(f.eventsOf("answers")); n != 1 {
				t.Errorf("%d answers stored, want 1", n)
			}
		})
	}
}

// Sequence numbers in the kept events lift NextSeq on Open, even when
// link.json lost its own.
func TestOpenLiftsNextSeqFromParked(t *testing.T) {
	f, c, st, clk := linked(t)
	for range 3 {
		c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	}
	loseLink(t, f, c)
	c.Close()
	c2 := reopen(f, st, clk)
	defer c2.Close()
	c2.mu.Lock()
	defer c2.mu.Unlock()
	if want := maxParkedSeq(c2.parked) + 1; want < 4 || c2.st.NextSeq < want {
		t.Errorf("NextSeq %d, want at least %d", c2.st.NextSeq, want)
	}
}

// Parking twice for one learner merges, in sequence order, and keeps
// one entry.
func TestParkTwiceMerges(t *testing.T) {
	_, c, _, _ := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	c.mu.Lock()
	c.park()
	c.q = queue{}
	c.mu.Unlock()
	c.Answer("fr", cat, "practice", words.Answer{Tier: words.Perfect})
	c.mu.Lock()
	defer c.mu.Unlock()
	c.park()
	if len(c.parked) != 1 || len(c.parked[0].Queue.Answers) != 2 {
		t.Fatalf("parked: %+v", c.parked)
	}
	a := c.parked[0].Queue.Answers
	if a[0].Seq >= a[1].Seq {
		t.Errorf("not in sequence order: %d, %d", a[0].Seq, a[1].Seq)
	}
}

// me's last_seq of 0, or lower than NextSeq, never lowers NextSeq.
func TestMeLastSeqNeverLowers(t *testing.T) {
	for _, last := range []int64{0, 1} {
		f, c, _, _ := linked(t)
		for range 3 {
			c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
		}
		if err := c.Sync(context.Background()); err != nil {
			t.Fatal(err)
		}
		f.mu.Lock()
		f.meSeq, f.lastSeq = true, last
		f.mu.Unlock()
		c.mu.Lock()
		before := c.st.NextSeq
		c.mu.Unlock()
		if err := c.Sync(context.Background()); err != nil {
			t.Fatal(err)
		}
		c.mu.Lock()
		got := c.st.NextSeq
		c.mu.Unlock()
		if got != before {
			t.Errorf("last_seq %d: NextSeq %d, was %d", last, got, before)
		}
	}
}

// invalid_token is a link the server ended (a grown-up unlinked the
// game, or a family withdrew the progress consent and the server deleted
// the learner's devices): the game unlinks, and the events it hadn't sent
// are dropped at once, on disk too, and never sent after a new link, even
// across a restart (#113).
func TestInvalidTokenUnlinksAndDrops(t *testing.T) {
	f, c, st, clk := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	c.Session(Session{Start: clk.now(), Mode: "practice", Lang: "fr", Secs: 60})
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	if !st.has(queueFile) {
		t.Fatal("no queue file to drop")
	}
	f.mu.Lock()
	f.refresh = "hwr_other" // the server knows no such token: invalid_token
	f.mu.Unlock()
	c.mu.Lock()
	c.st.AccessExp = time.Time{}
	c.mu.Unlock()
	if err := c.Sync(context.Background()); !errors.Is(err, ErrUnlinked) {
		t.Fatalf("sync: %v", err)
	}
	if s := c.Status(); s.Linked || s.Pending != 0 || s.Note != endedNote {
		t.Errorf("status: %+v", s)
	}
	if st.has(parkedFile) || st.has(queueFile) {
		t.Error("events kept on disk after invalid_token")
	}
	c.Close()
	c2 := reopen(f, st, clk)
	relink(t, f, c2)
	if n, m := len(f.eventsOf("answers")), len(f.eventsOf("sessions")); n != 0 || m != 0 {
		t.Errorf("%d answers and %d sessions from before the link ended sent after a new link", n, m)
	}
}

// invalid_token also drops what was parked for the same learner after an
// earlier lost link; another learner's parked events stay theirs.
func TestInvalidTokenDropsLearnersParked(t *testing.T) {
	f, c, st, clk := linked(t)
	c.mu.Lock()
	me := c.st.Me.Learner.ID
	old := queue{Answers: []qAnswer{{Seq: 900}}}
	c.parked = []parked{{Learner: me, At: clk.now(), Queue: old}, {Learner: "lrn_other", At: clk.now(), Queue: old}}
	err := c.writeParked()
	c.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.refresh = "hwr_other"
	f.mu.Unlock()
	c.mu.Lock()
	c.st.AccessExp = time.Time{}
	c.mu.Unlock()
	if err := c.Sync(context.Background()); !errors.Is(err, ErrUnlinked) {
		t.Fatalf("sync: %v", err)
	}
	var ps []parked
	if err := json.Unmarshal(st.files[parkedFile], &ps); err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0].Learner != "lrn_other" {
		t.Errorf("parked after invalid_token: %+v", ps)
	}
}

// token_reused is the game's own pair gone stale (a refresh whose answer
// was lost): the events not sent are still parked for the learner.
func TestTokenReusedParks(t *testing.T) {
	f, c, st, _ := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	loseLink(t, f, c)
	if !st.has(parkedFile) {
		t.Error("events not kept after token_reused")
	}
}

// A queue file left with no link (an unlink that couldn't remove it) is
// dropped at start-up, so it is never sent as the next link's.
func TestUnlinkedQueueFileDroppedAtOpen(t *testing.T) {
	st := newMemStore()
	data, _ := json.Marshal(queue{Answers: []qAnswer{{Seq: 7}}})
	if err := st.Write(queueFile, data); err != nil {
		t.Fatal(err)
	}
	c := Open(Options{Store: st, Server: "http://127.0.0.1:1"})
	t.Cleanup(c.Close)
	if s := c.Status(); s.Pending != 0 {
		t.Errorf("pending %d", s.Pending)
	}
	if st.has(queueFile) {
		t.Error("queue file kept")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.st.NextSeq < 1 {
		t.Errorf("NextSeq %d", c.st.NextSeq)
	}
}

// A refresh that fails every try is not a lost link: nothing is parked,
// the queue stays, and the waits are the retry waits, not real ones.
func TestRefreshRetriesExhaustedKeepsLink(t *testing.T) {
	f, c, st, _ := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	var slept []time.Duration
	c.sleep = func(d time.Duration) { slept = append(slept, d) }
	c.mu.Lock()
	c.st.AccessExp = time.Time{}
	c.mu.Unlock()
	f.failNext("/api/v1/token", 503, 503, 503)
	if err := c.Sync(context.Background()); err == nil || errors.Is(err, ErrUnlinked) {
		t.Fatalf("sync: %v", err)
	}
	if s := c.Status(); !s.Linked || s.Pending != 1 {
		t.Errorf("status: %+v", s)
	}
	if st.has(parkedFile) {
		t.Error("events parked though the link was not lost")
	}
	if len(slept) != tries-1 || slept[0] != retryWait(1) || slept[1] != retryWait(2) {
		t.Errorf("waits %v", slept)
	}
}

// token_reused parks the events; an invalid_token after the next link but
// before its first /me (the learner not known yet) still drops them, so
// nothing from before is sent after a later link (#113, review).
func TestTokenReusedThenInvalidTokenDropsParked(t *testing.T) {
	f, c, st, clk := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	loseLink(t, f, c)
	if !st.has(parkedFile) {
		t.Fatal("not parked after token_reused")
	}
	// Link again but don't sync, so the parked events are not yet sent.
	f.mu.Lock()
	f.code = "QRST-VWXY"
	f.mu.Unlock()
	if err := c.LinkNow(context.Background(), "QRSTVWXY"); err != nil {
		t.Fatal(err)
	}
	revoke(f)
	if err := c.Sync(context.Background()); !errors.Is(err, ErrUnlinked) {
		t.Fatalf("sync: %v", err)
	}
	if st.has(parkedFile) {
		t.Errorf("parked events survive invalid_token: %s", st.files[parkedFile])
	}
	c.Close()
	c2 := reopen(f, st, clk)
	defer c2.Close()
	relink(t, f, c2)
	if n := len(f.eventsOf("answers")); n != 0 {
		t.Errorf("%d old answers sent after relink", n)
	}
}

// A refresh token past its expiry (a game offline for months) answers
// invalid_token too; that is not a link the server ended, so the events
// are parked and sent after the learner links again, with the lost note.
func TestExpiredRefreshParks(t *testing.T) {
	f, c, st, clk := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	c.mu.Lock()
	exp := c.st.RefreshExp
	c.mu.Unlock()
	clk.add(exp.Sub(clk.now()) + time.Minute)
	revoke(f)
	if err := c.Sync(context.Background()); !errors.Is(err, ErrUnlinked) {
		t.Fatalf("sync: %v", err)
	}
	if s := c.Status(); s.Note != lostNote {
		t.Errorf("note %q", s.Note)
	}
	if !st.has(parkedFile) {
		t.Fatal("events not parked after an expired refresh token")
	}
	relink(t, f, c)
	if n := len(f.eventsOf("answers")); n != 1 {
		t.Errorf("%d answers sent after relink, want 1", n)
	}
}

// failStore fails to read one file with an error other than not-exist.
type failStore struct {
	*memStore
	name string
}

func (s failStore) Read(name string) ([]byte, error) {
	if name == s.name {
		return nil, errors.New("disk error")
	}
	return s.memStore.Read(name)
}

// The queue is dropped at start-up only when the state says for sure the
// game isn't linked: not when link.json can't be read or parsed.
func TestUnreadableStateKeepsQueue(t *testing.T) {
	st := newMemStore()
	q, _ := json.Marshal(queue{Answers: []qAnswer{{Seq: 7}}})
	st.files[queueFile] = q
	st.files[stateFile] = []byte(`{"Refresh":"r"}`)
	c := Open(Options{Store: failStore{st, stateFile}, Server: "http://127.0.0.1:1"})
	c.mu.Lock()
	n := c.q.len()
	c.mu.Unlock()
	c.Close()
	if n != 1 || !st.has(queueFile) {
		t.Errorf("queue dropped (%d left) when link.json couldn't be read", n)
	}
	st.files[stateFile] = []byte("{damaged")
	c = Open(Options{Store: st, Server: "http://127.0.0.1:1"})
	c.mu.Lock()
	n = c.q.len()
	c.mu.Unlock()
	c.Close()
	if n != 1 || !st.has(queueFile) {
		t.Errorf("queue dropped (%d left) when link.json couldn't be parsed", n)
	}
	// Read and parsed with no tokens: unlinked for sure, so it goes.
	st.files[stateFile] = []byte(`{"NextSeq":3}`)
	c = Open(Options{Store: st, Server: "http://127.0.0.1:1"})
	c.Close()
	if st.has(queueFile) {
		t.Error("queue kept with link.json saying unlinked")
	}
}

// The ended note fits the Account window in at most two lines.
func TestEndedNoteFits(t *testing.T) {
	if n := len(endedNote); n > 2*66 {
		t.Errorf("endedNote is %d characters, room for %d", n, 2*66)
	}
}
