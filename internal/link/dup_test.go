package link

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/halpworld/halpwords/pkg/words"
)

// TestQueueAndParkedHoldSameEvents: a game that stopped between writing
// the queue and the parked file when a learner linked again has the same
// events in both. They are sent once, not twice in one batch (which the
// server refuses, and the game would then drop them all).
func TestQueueAndParkedHoldSameEvents(t *testing.T) {
	f, c, st, clk := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	c.Session(Session{Start: clk.now(), Mode: "practice", Lang: "fr", Secs: 60})
	loseLink(t, f, c)
	c.Close()
	var ps []parked
	if err := json.Unmarshal(st.files[parkedFile], &ps); err != nil || len(ps) != 1 {
		t.Fatalf("parked: %v, %d", err, len(ps))
	}
	c2 := reopen(f, st, clk)
	f.mu.Lock()
	f.code = "QRST-VWXY"
	f.mu.Unlock()
	if err := c2.LinkNow(context.Background(), "QRSTVWXY"); err != nil {
		t.Fatal(err)
	}
	// The game stops here, after the queue was written and before the
	// parked file was.
	data, _ := json.Marshal(ps[0].Queue)
	st.files[queueFile] = data
	c3 := reopen(f, st, clk)
	c3.mu.Lock()
	kept := len(c3.parked)
	c3.mu.Unlock()
	if kept != 0 || st.has(parkedFile) {
		t.Errorf("starting kept %d parked copies of queued events", kept)
	}
	if err := c3.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a, s := len(f.eventsOf("answers")), len(f.eventsOf("sessions")); a != 1 || s != 1 {
		t.Errorf("stored %d answers and %d sessions, want 1 and 1", a, s)
	}
	if st.has(parkedFile) || c3.Status().Pending != 0 {
		t.Error("the kept events weren't cleared once sent")
	}
}

// TestParkWrittenOnClose: events parked when the parked file couldn't be
// written are written when the game quits.
func TestParkWrittenOnClose(t *testing.T) {
	f, c, st, _ := linked(t)
	fs := &fullStore{memStore: st, full: true}
	c.mu.Lock()
	c.o.Store = fs
	c.mu.Unlock()
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	loseLink(t, f, c)
	if st.has(parkedFile) {
		t.Fatal("written to a full disk")
	}
	fs.full = false
	c.Close()
	if !st.has(parkedFile) || !st.private[parkedFile] {
		t.Fatal("the parked events weren't written on quitting")
	}
	var ps []parked
	if err := json.Unmarshal(st.files[parkedFile], &ps); err != nil || len(ps) != 1 || len(ps[0].Queue.Answers) != 1 {
		t.Errorf("parked %+v: %v", ps, err)
	}
}

// TestSweepParked: the game drops expired parked events in a learner's
// folder at start-up, even for a learner who never plays again.
func TestSweepParked(t *testing.T) {
	st := newMemStore()
	now := time.Date(2026, 9, 26, 14, 5, 9, 0, time.UTC)
	ps := []parked{
		{Learner: "lrn_old", At: now.Add(-parkedFor - time.Hour), Queue: queue{Answers: []qAnswer{{Seq: 1}}}},
		{Learner: "lrn_new", At: now.Add(-time.Hour), Queue: queue{Answers: []qAnswer{{Seq: 2}}}},
	}
	data, _ := json.Marshal(ps)
	st.WritePrivate(parkedFile, data)
	if err := SweepParked(st, now); err != nil {
		t.Fatal(err)
	}
	var got []parked
	if err := json.Unmarshal(st.files[parkedFile], &got); err != nil || len(got) != 1 || got[0].Learner != "lrn_new" || !st.private[parkedFile] {
		t.Fatalf("after a sweep: %+v, %v", got, err)
	}
	if err := SweepParked(st, now.Add(parkedFor)); err != nil {
		t.Fatal(err)
	}
	if st.has(parkedFile) {
		t.Error("all expired, and the file is left")
	}
	// A folder without a parked file is fine; a damaged one can never
	// be sent, and goes.
	if err := SweepParked(st, now); err != nil {
		t.Fatal(err)
	}
	st.files[parkedFile] = []byte("{")
	if err := SweepParked(st, now); err != nil || st.has(parkedFile) {
		t.Errorf("damaged file: %v, kept %v", err, st.has(parkedFile))
	}
	// Kept "in the future" (the clock went back): expired, or it would
	// never be.
	ps = []parked{
		{Learner: "lrn_future", At: now.Add(48 * time.Hour), Queue: queue{Answers: []qAnswer{{Seq: 3}}}},
		{Learner: "lrn_soon", At: now.Add(time.Hour), Queue: queue{Answers: []qAnswer{{Seq: 4}}}},
	}
	data, _ = json.Marshal(ps)
	st.WritePrivate(parkedFile, data)
	if err := SweepParked(st, now); err != nil {
		t.Fatal(err)
	}
	got = nil
	if err := json.Unmarshal(st.files[parkedFile], &got); err != nil || len(got) != 1 || got[0].Learner != "lrn_soon" {
		t.Errorf("future entries: %+v, %v", got, err)
	}
}

// TestParkedFromTheFutureExpires: Open drops parked events dated more
// than a day ahead (the clock went back), as the sweep does.
func TestParkedFromTheFutureExpires(t *testing.T) {
	f, _, st, clk := linked(t)
	ps := []parked{
		{Learner: "lrn_1", At: clk.now().Add(48 * time.Hour), Queue: queue{Answers: []qAnswer{{Seq: 90}}}},
		{Learner: "lrn_2", At: clk.now().Add(time.Hour), Queue: queue{Answers: []qAnswer{{Seq: 91}}}},
	}
	data, _ := json.Marshal(ps)
	st.WritePrivate(parkedFile, data)
	c := reopen(f, st, clk)
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.parked) != 1 || c.parked[0].Learner != "lrn_2" {
		t.Errorf("parked after opening: %+v", c.parked)
	}
}

// TestLostRefreshReplyWithGrace: the server swapped the refresh token but
// its reply was lost. The game asks again with the same token, which the
// server takes once inside its grace window: the game stays linked and
// sends its answers (halpworld/halpwords-server#67).
func TestLostRefreshReplyWithGrace(t *testing.T) {
	f, c, st, _ := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	f.mu.Lock()
	f.grace, f.loseReply = true, 1
	f.mu.Unlock()
	c.mu.Lock()
	c.st.AccessExp = time.Time{}
	c.mu.Unlock()
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s := c.Status(); !s.Linked || s.Pending != 0 || s.Note != "" {
		t.Errorf("status: %+v", s)
	}
	if n := len(f.eventsOf("answers")); n != 1 {
		t.Errorf("%d answers stored", n)
	}
	if st.has(parkedFile) {
		t.Error("parked while still linked")
	}
	// Without the grace window, the same lost reply ends the link, and
	// the answers wait for the learner.
	f.mu.Lock()
	f.grace, f.loseReply = false, 1
	f.mu.Unlock()
	c.Answer("fr", cat, "practice", words.Answer{Tier: words.Perfect})
	c.mu.Lock()
	c.st.AccessExp = time.Time{}
	c.mu.Unlock()
	if err := c.Sync(context.Background()); !errors.Is(err, ErrUnlinked) {
		t.Fatalf("sync: %v", err)
	}
	if !st.has(parkedFile) {
		t.Error("the answer not sent wasn't kept")
	}
}

// TestMeLastSeqOutOfRange: a last_seq below 0 or past 1<<53 (more than a
// JSON number holds exactly) is ignored, not taken as the next number.
func TestMeLastSeqOutOfRange(t *testing.T) {
	for _, last := range []int64{-5, 1<<53 + 1, 1 << 62} {
		f, c, _, _ := linked(t)
		c.mu.Lock()
		before := c.st.NextSeq
		c.mu.Unlock()
		f.mu.Lock()
		f.meSeq, f.lastSeq = true, last
		f.mu.Unlock()
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

// TestSignInWrittenOnClose: brand-new tokens that couldn't be written
// when linking are written when the game quits.
func TestSignInWrittenOnClose(t *testing.T) {
	_, c, st, _ := setup(t)
	fs := &fullStore{memStore: st, full: true}
	c.mu.Lock()
	c.o.Store = fs
	c.mu.Unlock()
	if err := c.LinkNow(context.Background(), "abcd efgh"); err != nil {
		t.Fatal(err)
	}
	if st.has(stateFile) {
		t.Fatal("written to a full disk")
	}
	fs.full = false
	c.Close()
	var saved state
	if err := json.Unmarshal(st.files[stateFile], &saved); err != nil || !saved.linked() || saved.Refresh != c.st.Refresh {
		t.Errorf("saved %+v: %v", saved, err)
	}
}
