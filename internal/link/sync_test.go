package link

import (
	"context"
	"testing"
	"time"

	"github.com/halpworld/halpwords/pkg/words"
)

// reopen opens the client again from its files, as the game does at its
// next start.
func reopen(f *fake, st *memStore, clk *clock) *Client {
	c := Open(Options{Store: st, Server: f.srv.URL, OwnDir: "words", Now: clk.now})
	c.sleep = func(time.Duration) {}
	return c
}

// TestQuitThenPlay: quitting sends the queue, and the next session's
// answers don't reuse its sequence numbers (halpworld/halpwords#54).
func TestQuitThenPlay(t *testing.T) {
	f, c, st, clk := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	c.Close() // quitting sends what is queued
	if n := len(f.eventsOf("answers")); n != 1 {
		t.Fatalf("after close: %d stored", n)
	}
	c2 := reopen(f, st, clk)
	c2.Answer("fr", cat, "practice", words.Answer{Tier: words.Perfect})
	if err := c2.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(f.eventsOf("answers")); n != 2 {
		t.Errorf("%d answers stored, want 2: the second session reused a sequence number", n)
	}
}

// TestUploadSavedWhenSyncFailsLater: events sent in a sync that fails at
// a later step still move the saved sequence number on.
func TestUploadSavedWhenSyncFailsLater(t *testing.T) {
	f, c, st, clk := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	f.failNext("/api/v1/lists", 500, 500, 500)
	if err := c.Sync(context.Background()); err == nil {
		t.Fatal("sync worked with the lists failing")
	}
	if n := len(f.eventsOf("answers")); n != 1 {
		t.Fatalf("%d stored", n)
	}
	c2 := reopen(f, st, clk)
	c2.Answer("fr", cat, "practice", words.Answer{Tier: words.Perfect})
	if err := c2.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(f.eventsOf("answers")); n != 2 {
		t.Errorf("%d answers stored, want 2", n)
	}
}

// TestMeLastSeq: a server that says the device's last_seq in me lifts a
// sequence number saved too low (by an older game) before anything is
// sent; one that doesn't say changes nothing.
func TestMeLastSeq(t *testing.T) {
	f, c, st, clk := linked(t)
	f.mu.Lock()
	f.meSeq = true
	f.mu.Unlock()
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	// An older game quit without saving the sequence number.
	c.mu.Lock()
	c.st.NextSeq = 1
	c.saveState()
	c.mu.Unlock()
	c2 := reopen(f, st, clk)
	if err := c2.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	c2.Answer("fr", cat, "practice", words.Answer{Tier: words.Perfect})
	if err := c2.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(f.eventsOf("answers")); n != 2 {
		t.Errorf("%d answers stored, want 2", n)
	}
	// A server without last_seq: the number stays as it is.
	f.mu.Lock()
	f.meSeq = false
	f.mu.Unlock()
	c2.mu.Lock()
	seq := c2.st.NextSeq
	c2.mu.Unlock()
	if err := c2.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	c2.mu.Lock()
	defer c2.mu.Unlock()
	if c2.st.NextSeq != seq {
		t.Errorf("NextSeq %d, want %d", c2.st.NextSeq, seq)
	}
}
