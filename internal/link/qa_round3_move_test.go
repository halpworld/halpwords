package link

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/halpworld/halpwords/pkg/words"
)

// fullStore swaps c's store for one with no room.
func qa3Full(c *Client, st *memStore) *limitStore {
	ls := &limitStore{memStore: st, limit: storeUsed(st)}
	c.mu.Lock()
	c.o.Store = ls
	c.mu.Unlock()
	return ls
}

// TestQA3FullLostFreedSaveRelink is the whole path: the storage is full,
// the link is lost, room is freed, Save writes the parked file, the same
// client links again, and the events are sent once, with the queue file,
// the parked file and the old tokens gone.
func TestQA3FullLostFreedSaveRelink(t *testing.T) {
	f, c, st, clk := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	refresh := c.st.Refresh
	ls := qa3Full(c, st)
	loseLink(t, f, c)
	// Nothing could be written: the files that hold the events stay.
	if st.has(parkedFile) {
		t.Fatal("parked file written on full storage")
	}
	if !st.has(queueFile) || !st.has(stateFile) {
		t.Fatalf("queue file %v, state file %v kept; want both", st.has(queueFile), st.has(stateFile))
	}
	if !c.parkedUnsaved || !c.unlinkPending {
		t.Fatalf("parkedUnsaved %v unlinkPending %v", c.parkedUnsaved, c.unlinkPending)
	}
	// Room is freed.
	ls.limit = 1 << 20
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	if !st.has(parkedFile) || !st.private[parkedFile] {
		t.Fatal("parked file not written (privately) by Save")
	}
	if st.has(queueFile) {
		t.Error("queue file left beside the parked file")
	}
	if c.parkedUnsaved || c.unlinkPending {
		t.Errorf("parkedUnsaved %v unlinkPending %v after Save", c.parkedUnsaved, c.unlinkPending)
	}
	if refresh != "" && bytes.Contains(st.files[stateFile], []byte(refresh)) {
		t.Error("the dead refresh token is still in link.json")
	}
	relink(t, f, c)
	if n := len(f.eventsOf("answers")); n != 1 {
		t.Fatalf("%d answers sent, want 1", n)
	}
	if st.has(parkedFile) {
		t.Error("parked file left after sending")
	}
	if p := c.Status().Pending; p != 0 {
		t.Errorf("%d events pending", p)
	}
	// Nothing is sent twice by a later sync or a restart.
	c.Close()
	c2 := reopen(f, st, clk)
	if c2.Linked() {
		if err := c2.Sync(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(f.eventsOf("answers")); n != 1 {
		t.Errorf("%d answers after a restart, want 1", n)
	}
	if st.has(parkedFile) {
		t.Error("parked file reappeared")
	}
}

// TestQA3RelinkWhileParkedUnsaved: the learner links again before the
// parked file could be written: the events go once, and the parked file
// is never written (nothing is kept) when room comes.
func TestQA3RelinkWhileParkedUnsaved(t *testing.T) {
	f, c, st, clk := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	ls := qa3Full(c, st)
	loseLink(t, f, c)
	ls.limit = storeUsed(st) + 1<<20
	relink(t, f, c)
	if n := len(f.eventsOf("answers")); n != 1 {
		t.Fatalf("%d answers sent, want 1", n)
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	c.Close()
	if st.has(parkedFile) {
		t.Error("parked file left after the events were sent")
	}
	c2 := reopen(f, st, clk)
	if c2.Linked() {
		c2.Sync(context.Background())
	}
	if n := len(f.eventsOf("answers")); n != 1 {
		t.Errorf("%d answers after a restart, want 1", n)
	}
}

// TestQA3RestartAfterFailedParkStillFull: the game is started again on the
// kept files while the storage is still full, loses the link again, and
// only later, with room, writes the events: they are sent once.
func TestQA3RestartAfterFailedParkStillFull(t *testing.T) {
	f, c, st, clk := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	qa3Full(c, st)
	loseLink(t, f, c)
	// First restart, still full.
	ls := &limitStore{memStore: st, limit: storeUsed(st)}
	c2 := Open(Options{Store: ls, Server: f.srv.URL, OwnDir: "words", Now: clk.now})
	c2.sleep = func(time.Duration) {}
	if !c2.Linked() {
		t.Fatal("the kept files don't link the game again")
	}
	if c2.Status().Pending != 1 {
		t.Fatalf("%d pending after restart, want 1", c2.Status().Pending)
	}
	loseLink(t, f, c2)
	if st.has(parkedFile) {
		t.Fatal("parked on full storage")
	}
	// A second restart, with room now.
	ls.limit = 1 << 20
	if n := sentAfterRestart(t, f, st, clk); n != 1 {
		t.Fatalf("%d answers stored, want 1", n)
	}
}

// TestQA3RestartWithRoomAfterFailedPark: the restart finds room, syncs
// with the dead tokens, and the events are sent once.
func TestQA3RestartWithRoomAfterFailedPark(t *testing.T) {
	f, c, st, clk := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	qa3Full(c, st)
	loseLink(t, f, c)
	c2 := reopen(f, st, clk)
	if c2.Linked() {
		loseLink(t, f, c2)
	}
	c2.Close()
	if !st.has(parkedFile) {
		t.Fatal("not parked with room")
	}
	c3 := reopen(f, st, clk)
	relink(t, f, c3)
	if n := len(f.eventsOf("answers")); n != 1 {
		t.Errorf("%d answers stored, want 1", n)
	}
}
