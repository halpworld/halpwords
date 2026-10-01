package link

import (
	"context"
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

// Both codes of a finished refresh token unlink; the events are kept.
func TestInvalidTokenUnlinksAndParks(t *testing.T) {
	f, c, st, _ := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	f.mu.Lock()
	f.refresh = "hwr_other" // the server knows no such token: invalid_token
	f.mu.Unlock()
	c.mu.Lock()
	c.st.AccessExp = time.Time{}
	c.mu.Unlock()
	if err := c.Sync(context.Background()); !errors.Is(err, ErrUnlinked) {
		t.Fatalf("sync: %v", err)
	}
	if !st.has(parkedFile) {
		t.Error("events not kept after invalid_token")
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
