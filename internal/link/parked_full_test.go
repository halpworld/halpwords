package link

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/halpworld/halpwords/pkg/words"
)

// limitStore is a store with room for limit bytes, as a browser's local
// storage: a write that would go over fails; removing frees room. With
// limit 0 nothing can be written.
type limitStore struct {
	*memStore
	limit int
}

func (s *limitStore) room(name string, n int) error {
	used := n
	s.mu.Lock()
	for k, v := range s.files {
		if k != name {
			used += len(v)
		}
	}
	s.mu.Unlock()
	if used > s.limit {
		return errors.New("quota exceeded")
	}
	return nil
}

func (s *limitStore) Write(name string, data []byte) error {
	if err := s.room(name, len(data)); err != nil {
		return err
	}
	return s.memStore.Write(name, data)
}

func (s *limitStore) WritePrivate(name string, data []byte) error {
	if err := s.room(name, len(data)); err != nil {
		return err
	}
	return s.memStore.WritePrivate(name, data)
}

// storeUsed is the bytes in st.
func storeUsed(st *memStore) int {
	n := 0
	for _, d := range st.files {
		n += len(d)
	}
	return n
}

// sentAfterRestart starts the game again on st (a tab opened later), links
// it again and returns how many answers the server then has.
func sentAfterRestart(t *testing.T, f *fake, st Store, clk *clock) int {
	t.Helper()
	c := Open(Options{Store: st, Server: f.srv.URL, OwnDir: "words", Now: clk.now})
	c.sleep = func(time.Duration) {}
	if c.Linked() {
		// The unlink couldn't be written either: the dead tokens are
		// still there, and the link is lost again.
		loseLink(t, f, c)
	}
	relink(t, f, c)
	return len(f.eventsOf("answers"))
}

// TestParkSurvivesFullStorageWithoutClose: with the storage full, losing
// the link must not drop the queue file, or a web game (which never gets
// to Close) loses the events when the tab closes.
func TestParkSurvivesFullStorageWithoutClose(t *testing.T) {
	f, c, st, clk := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	ls := &limitStore{memStore: st, limit: storeUsed(st)} // no room for more
	c.mu.Lock()
	c.o.Store = ls
	c.mu.Unlock()
	loseLink(t, f, c)
	// The tab closes here: no Close.
	if n := sentAfterRestart(t, f, st, clk); n != 1 {
		t.Fatalf("%d answers stored after a restart, want 1: the events were lost", n)
	}
}

// TestParkFreesRoomByDroppingQueue: when the storage has room for the
// queue but not for the queue and the parked file, the queue file goes
// first, and the parked file takes its place.
func TestParkFreesRoomByDroppingQueue(t *testing.T) {
	f, c, st, clk := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	// How big the parked file will be.
	f0, c0, st0, _ := linked(t)
	c0.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	loseLink(t, f0, c0)
	parkedSize := len(st0.files[parkedFile])
	// Room for the parked file only once the queue file is gone.
	ls := &limitStore{memStore: st, limit: storeUsed(st) - len(st.files[queueFile]) + parkedSize}
	c.mu.Lock()
	c.o.Store = ls
	c.mu.Unlock()
	loseLink(t, f, c)
	if !st.has(parkedFile) {
		t.Fatal("the events weren't parked")
	}
	if st.has(queueFile) {
		t.Error("the queue file is left beside the parked file")
	}
	if n := sentAfterRestart(t, f, st, clk); n != 1 {
		t.Fatalf("%d answers stored after a restart, want 1", n)
	}
}

// TestParkRetriedOnSave: a parked file that couldn't be written is
// written by Save and TrySave (a web page's hide and close handlers), not
// only by Close, and the queue file kept meanwhile then goes.
func TestParkRetriedOnSave(t *testing.T) {
	for _, try := range []bool{false, true} {
		f, c, st, clk := linked(t)
		c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
		if err := c.Save(); err != nil {
			t.Fatal(err)
		}
		ls := &limitStore{memStore: st}
		c.mu.Lock()
		c.o.Store = ls
		c.mu.Unlock()
		loseLink(t, f, c)
		if st.has(parkedFile) {
			t.Fatal("parked on full storage")
		}
		ls.limit = 1 << 20
		var err error
		if try {
			err = c.TrySave()
		} else {
			err = c.Save()
		}
		if err != nil {
			t.Fatalf("try %v: %v", try, err)
		}
		if !st.has(parkedFile) || !st.private[parkedFile] {
			t.Errorf("try %v: the parked events weren't written", try)
		}
		if st.has(queueFile) {
			t.Errorf("try %v: the queue file is left beside the parked file", try)
		}
		if n := sentAfterRestart(t, f, st, clk); n != 1 {
			t.Errorf("try %v: %d answers stored, want 1", try, n)
		}
	}
}

// A link lost with full storage keeps the old queue file on disk. Another
// learner linking must not leave it there: a tab that closed before the
// next save would send the first learner's events as the second.
func TestKeptQueueGoesWhenAnotherLinks(t *testing.T) {
	f, c, st, _ := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	ls := &limitStore{memStore: st, limit: storeUsed(st)}
	c.mu.Lock()
	c.o.Store = ls
	c.mu.Unlock()
	loseLink(t, f, c)
	if !st.has(queueFile) {
		t.Fatal("test setup: the queue file wasn't kept")
	}
	f.mu.Lock()
	f.code = "QRST-VWXY"
	f.mu.Unlock()
	if err := c.LinkNow(context.Background(), "QRSTVWXY"); err != nil {
		t.Fatal(err)
	}
	// No save, no Close: the tab closes here.
	if st.has(queueFile) {
		t.Error("the first learner's queue file is still there after another linked")
	}
	c.mu.Lock()
	kept := len(c.parked)
	c.mu.Unlock()
	if kept != 1 {
		t.Errorf("%d parked entries in memory, want 1", kept)
	}
}

// Quitting with unwritten tokens must not write the unlinked state over
// the one kept for the events that couldn't be parked.
func TestCloseKeepsLinkStateWhilePending(t *testing.T) {
	f, c, st, clk := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	ls := &limitStore{memStore: st, limit: storeUsed(st)}
	c.mu.Lock()
	c.o.Store = ls
	c.stateUnsaved = true // new tokens that were never written
	c.mu.Unlock()
	loseLink(t, f, c)
	c.Close()
	var saved state
	if err := json.Unmarshal(st.files[stateFile], &saved); err != nil || !saved.linked() {
		t.Fatalf("link.json after Close: %+v, %v: the link was forgotten", saved, err)
	}
	if !st.has(queueFile) {
		t.Fatal("the queue file went")
	}
	if n := sentAfterRestart(t, f, st, clk); n != 1 {
		t.Fatalf("%d answers stored after a restart, want 1", n)
	}
}
