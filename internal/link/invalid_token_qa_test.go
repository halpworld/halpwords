package link

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/halpworld/halpwords/pkg/words"
)

// revoke makes the server forget the tokens: every call is a 401 and the
// refresh answers invalid_token (a grown-up unlinked, consent withdrawn).
func revoke(f *fake) {
	f.mu.Lock()
	f.access, f.refresh = "", ""
	f.mu.Unlock()
}

// The access token is still fresh by the clock, so the first 401 comes
// from the events POST itself: invalid_token in the middle of a sync with
// answers and a session in flight.
func TestInvalidTokenMidSyncDropsEverything(t *testing.T) {
	f, c, st, clk := linked(t)
	for i := 0; i < 5; i++ {
		c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	}
	c.Session(Session{Start: clk.now(), Mode: "practice", Lang: "fr", Secs: 60})
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	revoke(f)
	if err := c.Sync(context.Background()); !errors.Is(err, ErrUnlinked) {
		t.Fatalf("sync: %v", err)
	}
	if s := c.Status(); s.Linked || s.Pending != 0 {
		t.Errorf("status %+v", s)
	}
	if st.has(queueFile) || st.has(parkedFile) {
		t.Error("events survive on disk")
	}
	// Answers after the loss are not recorded either (no link).
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	if s := c.Status(); s.Pending != 0 {
		t.Errorf("answer while unlinked queued: %+v", s)
	}
	c.Close()
	c2 := reopen(f, st, clk)
	defer c2.Close()
	relink(t, f, c2)
	if n, m := len(f.eventsOf("answers")), len(f.eventsOf("sessions")); n != 0 || m != 0 {
		t.Errorf("old events sent after relink: %d answers %d sessions", n, m)
	}
	// And new events from the new link do go out, with no seq collision.
	c2.Answer("fr", cat, "practice", words.Answer{Tier: words.Perfect})
	if err := c2.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(f.eventsOf("answers")); n != 1 {
		t.Errorf("%d new answers stored, want 1", n)
	}
}

// Several learners' parked entries: only the linked learner's go.
func TestInvalidTokenKeepsOtherLearnersParkedAcrossRestart(t *testing.T) {
	f, c, st, clk := linked(t)
	c.mu.Lock()
	old := queue{Answers: []qAnswer{{Seq: 901}}}
	c.parked = []parked{
		{Learner: "lrn_a", At: clk.now(), Queue: old},
		{Learner: "lrn_1", At: clk.now(), Queue: old},
		{Learner: "lrn_1", At: clk.now(), Queue: old},
		{Learner: "lrn_b", At: clk.now(), Queue: old},
	}
	err := c.writeParked()
	c.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	revoke(f)
	if err := c.Sync(context.Background()); !errors.Is(err, ErrUnlinked) {
		t.Fatalf("sync: %v", err)
	}
	c.Close()
	c2 := reopen(f, st, clk)
	defer c2.Close()
	c2.mu.Lock()
	defer c2.mu.Unlock()
	var got []string
	for _, p := range c2.parked {
		got = append(got, p.Learner)
	}
	if len(got) != 2 || got[0] != "lrn_a" || got[1] != "lrn_b" {
		t.Errorf("parked after restart: %v", got)
	}
}

// failRemoveStore cannot remove the queue file: an unlink that couldn't
// remove it (the case Open cleans up).
type failRemoveStore struct{ *memStore }

func (s failRemoveStore) Remove(name string) error {
	if name == queueFile {
		return errors.New("disk says no")
	}
	return s.memStore.Remove(name)
}

func TestInvalidTokenWithUndeletableQueueFile(t *testing.T) {
	f := newFake(t)
	base := newMemStore()
	clk := &clock{t: time.Date(2026, 9, 26, 14, 5, 9, 0, time.UTC)}
	open := func(st Store) *Client {
		c := Open(Options{Store: st, Server: f.srv.URL, OwnDir: "words", Now: clk.now})
		c.sleep = func(time.Duration) {}
		return c
	}
	c := open(failRemoveStore{base})
	f.setLists(wireList{ID: "lst_animals", Version: 3, Title: "Animals", Language: "fr", Words: 2, Text: animals})
	if err := c.LinkNow(context.Background(), "abcd efgh"); err != nil {
		t.Fatal(err)
	}
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	revoke(f)
	if err := c.Sync(context.Background()); !errors.Is(err, ErrUnlinked) {
		t.Fatalf("sync: %v", err)
	}
	c.Close() // must not resurrect the queue as a linked one
	// A restart on a healthy disk: nothing of the old queue is sent.
	c2 := open(base)
	defer c2.Close()
	if s := c2.Status(); s.Linked || s.Pending != 0 {
		t.Errorf("after restart: %+v", s)
	}
	f.mu.Lock()
	f.code = "QRST-VWXY"
	f.mu.Unlock()
	if err := c2.LinkNow(context.Background(), "QRSTVWXY"); err != nil {
		t.Fatal(err)
	}
	if err := c2.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(f.eventsOf("answers")); n != 0 {
		t.Errorf("%d old answers sent after relink", n)
	}
}

// A linked game's queue file is kept across a crash.
func TestOpenKeepsLinkedQueueFile(t *testing.T) {
	f, c, st, clk := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	// Quit without Close (a crash): the queue file is on disk.
	c2 := reopen(f, st, clk)
	defer c2.Close()
	if s := c2.Status(); !s.Linked || s.Pending != 1 {
		t.Errorf("linked restart lost the queue: %+v", s)
	}
}
