package link

import (
	"context"
	"encoding/json"
	"errors"
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

// linkedOn links a client that keeps its files in st.
func linkedOn(t *testing.T, st Store) *Client {
	t.Helper()
	f := newFake(t)
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
	return c
}

// A save writing the queue while the game is unlinked doesn't leave the
// old learner's queue on disk after the unlink removed it (#80).
func TestQueueNotWrittenAfterUnlink(t *testing.T) {
	st := &slowQueueStore{memStore: newMemStore(), entered: make(chan struct{}), release: make(chan struct{})}
	c := linkedOn(t, st)
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

// A save that finds another queue write on its way waits for it without
// holding c.mu, so the game loop isn't held up meanwhile.
func TestSaveWaitsForWriteWithoutLock(t *testing.T) {
	st := &slowQueueStore{memStore: newMemStore(), entered: make(chan struct{}), release: make(chan struct{})}
	c := linkedOn(t, st)
	c.Answer("fr", own, "practice", words.Answer{Tier: words.Perfect})
	st.mu.Lock()
	st.armed = true
	st.mu.Unlock()
	first := make(chan error, 1)
	go func() { first <- c.Save() }()
	<-st.entered
	c.Answer("fr", own, "practice", words.Answer{Tier: words.Correct}) // dirty again
	second := make(chan error, 1)
	go func() { second <- c.Save() }()
	time.Sleep(50 * time.Millisecond) // the second save waits for the first
	free := make(chan struct{})
	go func() { c.Status(); close(free) }()
	select {
	case <-free:
	case <-time.After(2 * time.Second):
		close(st.release)
		t.Fatal("c.mu was held while a queue write was on its way")
	}
	close(st.release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	data, err := st.Read(queueFile)
	if err != nil {
		t.Fatal(err)
	}
	var q queue
	if err := json.Unmarshal(data, &q); err != nil {
		t.Fatal(err)
	}
	n := len(q.Answers)
	for _, tt := range q.Totals {
		n += tt.Answers
	}
	if n != 2 {
		t.Errorf("the queue file holds %d answers, want both", n)
	}
}

// failQueueStore fails writes of the queue file while fail is set, calling
// during first.
type failQueueStore struct {
	*memStore
	mu     sync.Mutex
	fail   bool
	during func()
}

func (s *failQueueStore) Write(name string, data []byte) error {
	s.mu.Lock()
	fail, during := s.fail && name == queueFile, s.during
	s.mu.Unlock()
	if fail {
		if during != nil {
			during()
		}
		return errors.New("disk full")
	}
	return s.memStore.Write(name, data)
}

// A TrySave whose write fails while c.mu is taken still leaves the queue
// to be written at the next save.
func TestTrySaveFailedWriteStaysDirty(t *testing.T) {
	st := &failQueueStore{memStore: newMemStore()}
	c := linkedOn(t, st)
	c.Answer("fr", own, "practice", words.Answer{Tier: words.Perfect})
	st.mu.Lock()
	st.fail = true
	st.during = func() { c.mu.Lock() } // the game loop takes it meanwhile
	st.mu.Unlock()
	if err := c.TrySave(); err == nil {
		t.Fatal("the failed write wasn't reported")
	}
	c.mu.Unlock()
	st.mu.Lock()
	st.fail, st.during = false, nil
	st.mu.Unlock()
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	if !st.has(queueFile) {
		t.Error("the queue wasn't written at the next save")
	}
}
