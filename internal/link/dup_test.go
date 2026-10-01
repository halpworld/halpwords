package link

import (
	"context"
	"encoding/json"
	"testing"

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
