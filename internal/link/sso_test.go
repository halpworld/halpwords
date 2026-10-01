package link

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// ssoServer is a pretend server for signing in with a school account:
// the pupil signs in when signedIn is set, or is refused when refused is.
type ssoServer struct {
	mu       sync.Mutex
	started  map[string]any // the last start request
	polls    int
	signedIn bool
	refused  bool
	off      bool
	// notLinkable answers 403 not_linkable, as for a pupil whose school
	// hasn't given consent yet; the code stays good.
	notLinkable bool
	used        bool
	name        string
	// hang makes a poll wait this long before it answers; slowDown
	// answers slow_down; gate, when not nil, holds a poll until it is
	// closed (entered is told first).
	hang     time.Duration
	slowDown bool
	gate     chan struct{}
	entered  chan struct{}
}

func (f *ssoServer) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	hang, gate, entered := f.hang, f.gate, f.entered
	f.mu.Unlock()
	if r.URL.Path == "/api/v1/sso/token" {
		if entered != nil {
			entered <- struct{}{}
		}
		if gate != nil {
			<-gate
		}
		time.Sleep(hang)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var body map[string]any
	json.NewDecoder(r.Body).Decode(&body)
	if f.off {
		writeErr(w, http.StatusNotFound, codeSSOOff)
		return
	}
	switch r.URL.Path {
	case "/api/v1/sso/start":
		f.started = body
		writeJSON(w, http.StatusOK, map[string]any{"device_code": "hwsso_abc", "user_code": "ABCD-EFGH",
			"verification_uri": "https://halpwords.test/sso/device", "verification_uri_complete": "https://halpwords.test/sso/device?code=ABCD-EFGH",
			"expires_in": 600, "interval": 5})
	case "/api/v1/sso/token":
		f.polls++
		switch {
		case body["device_code"] != "hwsso_abc" || f.used:
			writeErr(w, http.StatusBadRequest, codeExpiredToken)
		case f.refused:
			writeErr(w, http.StatusForbidden, codeAccessDenied)
		case f.notLinkable:
			writeErr(w, http.StatusForbidden, codeNotLinkable)
		case f.slowDown:
			writeErr(w, http.StatusBadRequest, codeSlowDown)
		case !f.signedIn:
			writeErr(w, http.StatusBadRequest, codeAuthorizationPending)
		default:
			f.used = true
			f.name, _ = body["name"].(string)
			writeJSON(w, http.StatusOK, map[string]any{"device_id": "dev_1", "token_type": "Bearer", "access_token": "hwd_1",
				"expires_in": 3600, "refresh_token": "hwr_1", "refresh_expires_in": 86400})
		}
	default:
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

func ssoSetup(t *testing.T) (*ssoServer, *Client, *memStore, *clock) {
	t.Helper()
	f := &ssoServer{}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	st := newMemStore()
	clk := &clock{t: time.Date(2026, 9, 26, 14, 5, 9, 0, time.UTC)}
	c := Open(Options{Store: st, Server: srv.URL, Now: clk.now})
	c.sleep = func(time.Duration) {}
	return f, c, st, clk
}

func TestSSOSignIn(t *testing.T) {
	f, c, st, _ := ssoSetup(t)
	ctx := context.Background()
	if c.PendingSSO() != nil {
		t.Fatal("a sign-in on its way before starting")
	}
	code, err := c.StartSSO(ctx, "https://play.halpwords.test/")
	if err != nil {
		t.Fatal(err)
	}
	if code.UserCode != "ABCD-EFGH" || code.Every != 5*time.Second || code.VerifyURL == "" {
		t.Fatalf("code %+v", code)
	}
	if f.started["return_to"] != "https://play.halpwords.test/" {
		t.Errorf("start sent %v", f.started)
	}
	if err := c.PollSSO(ctx); !errors.Is(err, ErrSSOPending) {
		t.Fatalf("before signing in: %v", err)
	}
	// The web game comes back to a new page: the code is in link.json.
	c2 := Open(Options{Store: st, Server: c.server, Now: c.now})
	if p := c2.PendingSSO(); p == nil || p.DeviceCode != "hwsso_abc" {
		t.Fatalf("pending after a reload: %+v", p)
	}
	f.signedIn = true
	if err := c2.PollSSO(ctx); err != nil {
		t.Fatal(err)
	}
	if !c2.Linked() || c2.Way() != WaySSO || c2.KeepOnSignOut() || c2.PendingSSO() != nil {
		t.Fatalf("after signing in: linked %v, way %q", c2.Linked(), c2.Way())
	}
	if f.name == "" {
		t.Error("the game sent no name")
	}
	if _, err := c2.StartSSO(ctx, ""); !errors.Is(err, ErrLinked) {
		t.Errorf("starting when linked: %v", err)
	}
}

func TestSSORefusedAndExpired(t *testing.T) {
	f, c, st, clk := ssoSetup(t)
	ctx := context.Background()
	if _, err := c.StartSSO(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.started["return_to"]; ok {
		t.Error("a desktop game sent return_to")
	}
	f.refused = true
	if err := c.PollSSO(ctx); !errors.Is(err, ErrSSORefused) {
		t.Fatalf("refused: %v", err)
	}
	if c.PendingSSO() != nil || c.Linked() || st.has(stateFile) {
		t.Fatal("a refused sign-in is still on its way")
	}
	// A code that ran out isn't sent.
	f.refused = false
	if _, err := c.StartSSO(ctx, ""); err != nil {
		t.Fatal(err)
	}
	clk.add(11 * time.Minute)
	polls := f.polls
	if err := c.PollSSO(ctx); !errors.Is(err, ErrSSOExpired) || f.polls != polls {
		t.Fatalf("expired: %v, %d polls", err, f.polls-polls)
	}
	// Cancelling forgets the code.
	if _, err := c.StartSSO(ctx, ""); err != nil {
		t.Fatal(err)
	}
	c.CancelSSO()
	if c.PendingSSO() != nil {
		t.Fatal("cancelled but still on its way")
	}
	// A server without school accounts says so.
	f.off = true
	if _, err := c.StartSSO(ctx, ""); !errors.Is(err, ErrSSOOff) || Explain(err) == "" {
		t.Fatalf("off: %v", err)
	}
}

// TestSSONotLinkableKeepsCode: a pupil who can't be linked yet (403
// not_linkable) keeps the code until it runs out: once a grown-up sorts
// it out on the website, the next poll signs in.
func TestSSONotLinkableKeepsCode(t *testing.T) {
	f, c, _, clk := ssoSetup(t)
	ctx := context.Background()
	if _, err := c.StartSSO(ctx, ""); err != nil {
		t.Fatal(err)
	}
	f.signedIn, f.notLinkable = true, true
	if err := c.PollSSO(ctx); !errors.Is(err, ErrNotLinkable) {
		t.Fatalf("not linkable: %v", err)
	}
	if c.PendingSSO() == nil || c.Linked() {
		t.Fatal("the code was forgotten")
	}
	f.notLinkable = false
	if err := c.PollSSO(ctx); err != nil {
		t.Fatal(err)
	}
	if !c.Linked() {
		t.Fatal("not linked once linkable")
	}
	// Until it runs out.
	c.Unlink()
	f.used, f.notLinkable = false, true
	if _, err := c.StartSSO(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := c.PollSSO(ctx); !errors.Is(err, ErrNotLinkable) {
		t.Fatalf("not linkable: %v", err)
	}
	clk.add(11 * time.Minute)
	if err := c.PollSSO(ctx); !errors.Is(err, ErrSSOExpired) || c.PendingSSO() != nil {
		t.Fatalf("expired: %v", err)
	}
}

// A poll that times out is a poor connection, not a finished sign-in:
// the code stays until it runs out, and a later poll can still sign in.
func TestSSOPollTimeoutKeepsCode(t *testing.T) {
	f, c, _, _ := ssoSetup(t)
	ctx := context.Background()
	if _, err := c.StartSSO(ctx, ""); err != nil {
		t.Fatal(err)
	}
	f.hang = 300 * time.Millisecond
	c.hc.Timeout = 50 * time.Millisecond
	err := c.PollSSO(ctx)
	if err == nil || errors.Is(err, ErrSSOPending) {
		t.Fatalf("a slow poll: %v", err)
	}
	if c.PendingSSO() == nil {
		t.Fatal("one slow poll forgot the code")
	}
	// A poll cut off by the caller's own deadline keeps it too.
	short, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if err := c.PollSSO(short); err == nil || c.PendingSSO() == nil {
		t.Fatalf("a poll cut short: %v, pending %v", err, c.PendingSSO() != nil)
	}
	f.mu.Lock()
	f.hang, f.signedIn = 0, true
	f.mu.Unlock()
	c.hc.Timeout = 0
	if err := c.PollSSO(ctx); err != nil || !c.Linked() {
		t.Fatalf("after the connection came back: %v, linked %v", err, c.Linked())
	}
}

// When the code runs out while the connection is bad, the poll says so.
func TestSSOPollTimeoutAfterExpiry(t *testing.T) {
	f, c, _, clk := ssoSetup(t)
	ctx := context.Background()
	if _, err := c.StartSSO(ctx, ""); err != nil {
		t.Fatal(err)
	}
	f.hang = 300 * time.Millisecond
	c.hc.Timeout = 50 * time.Millisecond
	f.entered = make(chan struct{}, 4)
	go func() { <-f.entered; clk.add(11 * time.Minute) }()
	if err := c.PollSSO(ctx); !errors.Is(err, ErrSSOExpired) {
		t.Fatalf("a slow poll after the code ran out: %v", err)
	}
	if c.PendingSSO() != nil {
		t.Fatal("an expired code is still on its way")
	}
}

// slow_down makes the game ask less often, for the rest of the sign-in.
func TestSSOSlowDown(t *testing.T) {
	f, c, st, _ := ssoSetup(t)
	ctx := context.Background()
	if _, err := c.StartSSO(ctx, ""); err != nil {
		t.Fatal(err)
	}
	f.slowDown = true
	if err := c.PollSSO(ctx); !errors.Is(err, ErrSSOPending) {
		t.Fatalf("slow_down: %v", err)
	}
	p := c.PendingSSO()
	if p == nil || p.Every != 10*time.Second {
		t.Fatalf("after slow_down: %+v", p)
	}
	if err := c.PollSSO(ctx); !errors.Is(err, ErrSSOPending) {
		t.Fatal(err)
	}
	if p := c.PendingSSO(); p.Every != 15*time.Second {
		t.Fatalf("after two: %v", p.Every)
	}
	c2 := Open(Options{Store: st, Server: c.server, Now: c.now})
	if p := c2.PendingSSO(); p == nil || p.Every != 15*time.Second {
		t.Fatalf("after a reload: %+v", p)
	}
}

// Esc cancels the code while a poll is on its way: when the pupil had
// signed in meanwhile, the game must not end up linked.
func TestSSOCancelDuringPoll(t *testing.T) {
	f, c, _, _ := ssoSetup(t)
	ctx := context.Background()
	if _, err := c.StartSSO(ctx, ""); err != nil {
		t.Fatal(err)
	}
	f.signedIn = true
	f.gate, f.entered = make(chan struct{}), make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() { done <- c.PollSSO(ctx) }()
	<-f.entered
	c.CancelSSO()
	close(f.gate)
	if err := <-done; err == nil {
		t.Fatal("a cancelled sign-in went through")
	}
	if c.Linked() {
		t.Fatal("the game linked after the sign-in was cancelled")
	}
}
