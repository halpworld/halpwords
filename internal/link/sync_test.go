package link

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

// loseLink makes the server answer the game's next sync with
// token_reused, as after a refresh whose answer was lost: the game
// unlinks itself.
func loseLink(t *testing.T, f *fake, c *Client) {
	t.Helper()
	f.mu.Lock()
	f.used[f.refresh] = true
	f.mu.Unlock()
	f.failNext("/api/v1/me", 401)
	if err := c.Sync(context.Background()); !errors.Is(err, ErrUnlinked) {
		t.Fatalf("sync: %v", err)
	}
}

// relink links the game again with a new pairing code and syncs.
func relink(t *testing.T, f *fake, c *Client) {
	t.Helper()
	f.mu.Lock()
	f.code = "QRST-VWXY"
	f.mu.Unlock()
	if err := c.LinkNow(context.Background(), "QRSTVWXY"); err != nil {
		t.Fatal(err)
	}
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// TestLostLinkKeepsQueue: a game that loses its link keeps the events it
// hadn't sent, even across a restart, and sends them once the same
// learner links it again (halpworld/halpwords#39).
func TestLostLinkKeepsQueue(t *testing.T) {
	f, c, st, clk := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	c.Session(Session{Start: clk.now(), Mode: "practice", Lang: "fr", Secs: 60})
	loseLink(t, f, c)
	if s := c.Status(); s.Linked || s.Pending != 0 || s.Note == "" {
		t.Errorf("status: %+v", s)
	}
	if !st.has(parkedFile) || !st.private[parkedFile] {
		t.Fatal("the events not sent weren't kept, privately")
	}
	c.Close()
	c2 := reopen(f, st, clk)
	relink(t, f, c2)
	if len(f.eventsOf("answers")) != 1 || len(f.eventsOf("sessions")) != 1 {
		t.Errorf("stored %d answers and %d sessions, want 1 and 1", len(f.eventsOf("answers")), len(f.eventsOf("sessions")))
	}
	if st.has(parkedFile) || c2.Status().Pending != 0 {
		t.Error("the kept events weren't cleared once sent")
	}
}

// TestLostLinkQueueNotForAnotherLearner: kept events are never sent as
// another learner.
func TestLostLinkQueueNotForAnotherLearner(t *testing.T) {
	f, c, st, _ := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	loseLink(t, f, c)
	f.mu.Lock()
	f.me["learner"] = map[string]any{"id": "lrn_2", "display_name": "Brian", "avatar": map[string]any{"class": "knight", "colour": "#aabbcc"}, "languages": []string{"fr"}}
	f.mu.Unlock()
	relink(t, f, c)
	if n := len(f.eventsOf("answers")); n != 0 {
		t.Errorf("%d answers sent as another learner", n)
	}
	if !st.has(parkedFile) {
		t.Error("the first learner's events were thrown away")
	}
}

// TestLostLinkQueueExpires: kept events are thrown away after
// parkedFor.
func TestLostLinkQueueExpires(t *testing.T) {
	f, c, st, clk := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	loseLink(t, f, c)
	c.Close()
	clk.add(parkedFor + time.Hour)
	c2 := reopen(f, st, clk)
	if st.has(parkedFile) {
		t.Error("expired events kept on disk")
	}
	relink(t, f, c2)
	if n := len(f.eventsOf("answers")); n != 0 {
		t.Errorf("%d expired answers sent", n)
	}
	// Expired while the game runs: dropped before they would be sent.
	c2.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	loseLink(t, f, c2)
	clk.add(parkedFor + time.Hour)
	relink(t, f, c2)
	if n := len(f.eventsOf("answers")); n != 0 {
		t.Errorf("%d expired answers sent", n)
	}
}

// TestOther401FromTokenIsRetried: only invalid_token and token_reused end
// the link; any other 401 from POST /api/v1/token (a proxy, say) is a
// failure to try again later.
func TestOther401FromTokenIsRetried(t *testing.T) {
	f, c, _, _ := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	c.mu.Lock()
	c.st.AccessExp = time.Time{}
	c.mu.Unlock()
	f.failNext("/api/v1/token", 401) // unauthenticated
	if err := c.Sync(context.Background()); err == nil || errors.Is(err, ErrUnlinked) {
		t.Fatalf("sync: %v", err)
	}
	if s := c.Status(); !s.Linked || s.Pending != 1 || s.Note != "" {
		t.Fatalf("status: %+v", s)
	}
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(f.eventsOf("answers")); n != 1 {
		t.Errorf("%d answers stored", n)
	}
}

// fullStore is a store whose private files can't be written, as with a
// full disk or local storage.
type fullStore struct {
	*memStore
	full bool
}

func (s *fullStore) WritePrivate(name string, data []byte) error {
	if s.full {
		return errors.New("disk full")
	}
	return s.memStore.WritePrivate(name, data)
}

// TestRefreshUsedThoughNotSaved: new tokens that can't be written to
// disk are still used (the old refresh token no longer works), and
// written when the game quits.
func TestRefreshUsedThoughNotSaved(t *testing.T) {
	f, c, st, _ := linked(t)
	fs := &fullStore{memStore: st, full: true}
	c.mu.Lock()
	c.o.Store = fs
	c.st.AccessExp = time.Time{}
	refresh := c.st.Refresh
	c.mu.Unlock()
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	c.Sync(context.Background())
	c.mu.Lock()
	now, linked := c.st.Refresh, c.st.linked()
	c.mu.Unlock()
	if now == refresh || !linked {
		t.Fatalf("refresh token %q, linked %v: the new pair was dropped", now, linked)
	}
	if n := len(f.eventsOf("answers")); n != 1 {
		t.Errorf("%d answers stored", n)
	}
	fs.full = false
	c.Close()
	var saved state
	if err := json.Unmarshal(st.files[stateFile], &saved); err != nil || saved.Refresh != now {
		t.Errorf("saved refresh token %q, want %q (%v)", saved.Refresh, now, err)
	}
}

// TestMoveStateKeepsParked: when the learner whose link was lost signs in
// again on a new folder and the game moves that sign-in into their old
// folder (MoveState), the events kept there are sent, and new ones
// after them too.
func TestMoveStateKeepsParked(t *testing.T) {
	f, c, st, clk := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	loseLink(t, f, c)
	c.Close()
	other := newMemStore()
	n := reopen(f, other, clk)
	relink(t, f, n)
	n.Close()
	if len(f.eventsOf("answers")) != 0 {
		t.Fatal("kept events sent from another folder")
	}
	if err := MoveState(other, st); err != nil {
		t.Fatal(err)
	}
	c2 := reopen(f, st, clk)
	defer c2.Close()
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
	if st.has(parkedFile) {
		t.Error("the kept events weren't cleared once sent")
	}
}

// Quitting while offline with answers queued waits no longer than the
// close timeout, retries included.
func TestCloseOfflineDoesNotOutstayCloseTimeout(t *testing.T) {
	f, c, _, _ := linked(t)
	c.sleep = nil // the real, context-aware wait
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	f.failNext("/api/v1/events", 503, 503, 503, 503, 503, 503, 503, 503)
	start := time.Now()
	c.Close()
	if d := time.Since(start); d > closeTimeout+time.Second {
		t.Fatalf("Close took %v", d)
	}
}

// A sign-in is over as soon as the tokens are saved: the first sync goes
// on in the background, however slowly the server answers it.
func TestSignInDoesNotWaitForTheFirstSync(t *testing.T) {
	f := newFake(t)
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/link" {
			time.Sleep(300 * time.Millisecond)
		}
		f.srv.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(slow.Close)
	c := Open(Options{Store: newMemStore(), Server: slow.URL, OwnDir: "words"})
	c.SignIn(SignIn{Code: "abcd efgh"})
	if !c.Status().Signing {
		t.Fatal("not signing in right after asking")
	}
	start := time.Now()
	for c.Status().Signing {
		if time.Since(start) > 200*time.Millisecond {
			t.Fatalf("still signing in after %v: %+v", time.Since(start), c.Status())
		}
		time.Sleep(2 * time.Millisecond)
	}
	if st := c.Status(); !st.Linked || st.Err != nil {
		t.Fatalf("signed in: %+v", st)
	}
	// A wrong code ends it too, with the reason.
	c2 := Open(Options{Store: newMemStore(), Server: slow.URL, OwnDir: "words"})
	c2.SignIn(SignIn{Code: "wxyz wxyz"})
	for start := time.Now(); c2.Status().Signing; time.Sleep(2 * time.Millisecond) {
		if time.Since(start) > 200*time.Millisecond {
			t.Fatal("a refused sign-in never finished")
		}
	}
	if st := c2.Status(); st.Linked || !errors.Is(st.Err, ErrBadCode) {
		t.Fatalf("refused: %+v", st)
	}
	// Let the background sync end before the servers go.
	for c.Status().Busy {
		time.Sleep(10 * time.Millisecond)
	}
}

// Quitting (or moving to another learner) stops a first sync still on
// its way: it must not go on writing the folder after Close.
func TestCloseStopsTheFirstSync(t *testing.T) {
	f := newFake(t)
	gate := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/link" {
			select {
			case <-gate:
			case <-r.Context().Done():
				return
			}
		}
		f.srv.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(slow.Close)
	t.Cleanup(func() { close(gate) })
	c := Open(Options{Store: newMemStore(), Server: slow.URL, OwnDir: "words"})
	c.SignIn(SignIn{Code: "abcd efgh"})
	for start := time.Now(); c.Status().Signing; time.Sleep(2 * time.Millisecond) {
		if time.Since(start) > 2*time.Second {
			t.Fatal("never signed in")
		}
	}
	for start := time.Now(); !c.Status().Busy; time.Sleep(2 * time.Millisecond) {
		if time.Since(start) > 2*time.Second {
			t.Fatal("the first sync never started")
		}
	}
	c.Close()
	if st := c.Status(); st.Busy {
		t.Fatalf("a sync is still running after Close: %+v", st)
	}
}
