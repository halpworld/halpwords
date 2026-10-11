package link

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/halpworld/halpwords/pkg/words"
)

func (f *fake) suspend(on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.suspended = on
}

// TestSuspendedKeepsLink: while the server answers 403 account_suspended
// (halpwords-server#130) the game stays linked, keeps its queue, says so
// gently, syncs at the usual times, and picks up by itself when the
// suspension is lifted.
func TestSuspendedKeepsLink(t *testing.T) {
	f, c, st, _ := linked(t)
	f.suspend(true)
	for range 4 {
		c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
		if err := c.Sync(context.Background()); !errors.Is(err, ErrSuspended) {
			t.Fatalf("sync: %v", err)
		}
	}
	s := c.Status()
	if !s.Linked || !s.Suspended || s.Offline || s.Pending != 4 || s.Note != "" {
		t.Fatalf("status: %+v", s)
	}
	if got := Explain(s.Err); got != "can't sync right now: ask your grown-up" {
		t.Errorf("explained as %q", got)
	}
	if c.wait() != SyncEvery {
		t.Errorf("waits %v, not the usual %v", c.wait(), SyncEvery)
	}
	if c.AccessToken() == "" || len(c.Lists()) != 1 || !st.has(queueFile) || f.unlinked != 0 {
		t.Error("link, lists or queue lost")
	}
	if n := len(f.eventsOf("answers")); n != 0 {
		t.Errorf("%d answers stored while suspended", n)
	}

	f.suspend(false)
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s := c.Status(); !s.Linked || s.Suspended || s.Err != nil || s.Pending != 0 {
		t.Fatalf("after the suspension: %+v", s)
	}
	if n := len(f.eventsOf("answers")); n != 4 {
		t.Errorf("%d answers stored after the suspension", n)
	}
}

// TestSuspendedOnRefresh: a token refresh refused with 403
// account_suspended never unlinks; the tokens are kept for later.
func TestSuspendedOnRefresh(t *testing.T) {
	f, c, _, _ := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	c.mu.Lock()
	c.st.AccessExp = time.Time{}
	c.mu.Unlock()
	f.suspend(true)
	if err := c.Sync(context.Background()); !errors.Is(err, ErrSuspended) {
		t.Fatalf("sync: %v", err)
	}
	if s := c.Status(); !s.Linked || !s.Suspended || s.Pending != 1 || s.Note != "" {
		t.Fatalf("status: %+v", s)
	}
	f.suspend(false)
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s := c.Status(); s.Suspended || s.Pending != 0 {
		t.Fatalf("after the suspension: %+v", s)
	}
}

// TestIsSuspended: only 403 account_suspended is a suspension, and it
// ends the sync like a refused token (but never unlinks).
func TestIsSuspended(t *testing.T) {
	err := &Error{Status: 403, Code: codeAccountSuspended}
	if !isSuspended(err) || !stopsSync(context.Background(), err) {
		t.Error("account_suspended not seen")
	}
	if isSuspended(&Error{Status: 403, Code: codeNotLinkable}) {
		t.Error("not_linkable taken for a suspension")
	}
}
