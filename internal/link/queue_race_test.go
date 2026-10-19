package link

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/halpworld/halpwords/pkg/words"
)

// slowQueueStore holds a write of the queue file until release is closed,
// once armed.
type slowQueueStore struct {
	*memStore
	mu      sync.Mutex
	armed   bool
	entered chan struct{}
	release chan struct{}
}

func (s *slowQueueStore) Write(name string, data []byte) error {
	s.mu.Lock()
	hold := s.armed && name == queueFile
	s.armed = s.armed && !hold
	s.mu.Unlock()
	if hold {
		close(s.entered)
		<-s.release
	}
	return s.memStore.Write(name, data)
}

// A save writing the queue while the game is unlinked doesn't leave the
// old learner's queue on disk after the unlink removed it (#80).
func TestQueueNotWrittenAfterUnlink(t *testing.T) {
	f := newFake(t)
	st := &slowQueueStore{memStore: newMemStore(), entered: make(chan struct{}), release: make(chan struct{})}
	clk := &clock{t: time.Date(2026, 9, 26, 14, 5, 9, 0, time.UTC)}
	c := Open(Options{Store: st, Server: f.srv.URL, OwnDir: "words", Now: clk.now})
	t.Cleanup(c.Close)
	c.sleep = func(time.Duration) {}
	f.setLists(wireList{ID: "lst_animals", Version: 3, Title: "Animals", Language: "fr", Words: 2, Text: animals})
	if err := c.LinkNow(context.Background(), "abcd efgh"); err != nil {
		t.Fatal(err)
	}
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	c.Answer("fr", own, "practice", words.Answer{Tier: words.Perfect})
	st.mu.Lock()
	st.armed = true
	st.mu.Unlock()

	saved := make(chan error, 1)
	go func() { saved <- c.Save() }()
	<-st.entered // the save has its copy of the queue and is writing it
	unlinked := make(chan struct{})
	go func() { c.Unlink(); close(unlinked) }()
	// Give the unlink time to remove the file before the write ends, as
	// it did before the fix; with it, the unlink waits for the write.
	select {
	case <-unlinked:
	case <-time.After(50 * time.Millisecond):
	}
	close(st.release)
	if err := <-saved; err != nil {
		t.Fatal(err)
	}
	<-unlinked
	if c.Linked() {
		t.Fatal("still linked")
	}
	if st.has(queueFile) {
		t.Error("the queue file is on disk after the unlink")
	}
	// The next save doesn't bring it back either.
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	if st.has(queueFile) {
		t.Error("the queue file came back at the next save")
	}
}

// A TrySave finding a queue write on its way gives up rather than wait,
// and keeps the queue to write later.
func TestTrySaveBusyWhileQueueWrites(t *testing.T) {
	_, c, _, _ := linked(t)
	t.Cleanup(c.Close)
	c.Answer("fr", own, "practice", words.Answer{Tier: words.Perfect})
	c.qMu.Lock()
	err := c.TrySave()
	c.qMu.Unlock()
	if err == nil {
		t.Fatal("TrySave waited for the queue write")
	}
	c.mu.Lock()
	dirty := c.dirty
	c.mu.Unlock()
	if !dirty {
		t.Fatal("the queue was forgotten")
	}
	if err := c.TrySave(); err != nil {
		t.Fatal(err)
	}
}

// Unlink and a revoke at the same time as Close: their background runs
// are counted (bg.Add) under c.mu, so never while Close waits for them
// (#82). Run with -race.
func TestUnlinkRevokeDuringClose(t *testing.T) {
	for range 20 {
		_, c, _, _ := linked(t)
		var wg sync.WaitGroup
		wg.Add(3)
		go func() { defer wg.Done(); c.Close() }()
		go func() { defer wg.Done(); c.Unlink() }()
		go func() {
			defer wg.Done()
			c.mu.Lock()
			c.revoke(tokens{AccessToken: "a", RefreshToken: "r", ExpiresIn: 60})
			c.mu.Unlock()
		}()
		wg.Wait()
	}
}
