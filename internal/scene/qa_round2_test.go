package scene

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/pal"
)

// pollBackoff doubles up to a minute, never less than one interval, and
// cannot overflow for a large n.
func TestPollBackoffCap(t *testing.T) {
	s := time.Second
	for _, tc := range []struct {
		every time.Duration
		n     int
		want  time.Duration
	}{
		{5 * s, 0, 5 * s},
		{5 * s, 1, 10 * s},
		{5 * s, 2, 20 * s},
		{5 * s, 3, 40 * s},
		{5 * s, 4, time.Minute},
		{5 * s, 6, time.Minute},
		{5 * s, 1000, time.Minute},
		{2 * s, 5, time.Minute},
		{10 * s, 6, time.Minute},
		{2 * time.Minute, 0, 2 * time.Minute},
		{2 * time.Minute, 3, 2 * time.Minute}, // an interval above the cap is the floor
		{2 * time.Minute, 1000, 2 * time.Minute},
	} {
		if got := pollBackoff(tc.every, tc.n); got != tc.want {
			t.Errorf("pollBackoff(%v, %d) = %v, want %v", tc.every, tc.n, got, tc.want)
		}
	}
}

// slow_down seen by the screen: the next poll waits the longer interval.
func TestSignInSSOTakesSlowDown(t *testing.T) {
	var mu sync.Mutex
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/sso/start" {
			json.NewEncoder(w).Encode(map[string]any{"device_code": "hwsso_abc", "user_code": "ABCD-EFGH",
				"verification_uri": "https://halpwords.test/sso/device", "expires_in": 600, "interval": 5})
			return
		}
		polls++
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "slow_down", "message": "slower"}})
	}))
	t.Cleanup(srv.Close)
	openPage = func(string) error { return nil }
	ctx := testContext(t)
	ctx.Link = link.Open(link.Options{Store: &lockedFiles{m: memFiles{}}, Server: srv.URL})
	t.Cleanup(ctx.Link.Close)
	s := NewSignIn(ctx, "").(*SignIn)
	s.startSSO(ctx)
	s.answer(ctx, <-s.pending)
	ctx.Tick = s.nextPoll
	s.updateSSO(ctx)
	s.polled(ctx, waitPoll(t, s))
	if s.step != siSSO || s.sso.Every != 10*time.Second || s.nextPoll < ctx.Tick+ticks(10*time.Second) || s.nextPoll > ctx.Tick+ticks(11*time.Second) { // plus jitter
		t.Fatalf("every %v, next %d, tick %d", s.sso.Every, s.nextPoll, ctx.Tick)
	}
}

// Esc while the /link request is on its way does nothing: the sign-in
// goes through and the screen finishes it (the tokens are saved either
// way, so leaving would hide a game that is now signed in).
func TestSignInEscDuringLinkRequest(t *testing.T) {
	gate, in := make(chan struct{}), make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/link" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		in <- struct{}{}
		<-gate
		json.NewEncoder(w).Encode(map[string]any{"device_id": "dev_1", "token_type": "Bearer",
			"access_token": "hwd_1", "expires_in": 86400, "refresh_token": "hwr_1", "refresh_expires_in": 86400})
	}))
	t.Cleanup(srv.Close)
	esc := 0
	input.FakeKeys(t, func(k ebiten.Key) int {
		if k == ebiten.KeyEscape {
			return esc
		}
		return 0
	})
	ctx := testContext(t)
	ctx.Input = &input.State{}
	ctx.Link = link.Open(link.Options{Store: &lockedFiles{m: memFiles{}}, Server: srv.URL})
	t.Cleanup(ctx.Link.Close)
	s := NewSignIn(ctx, "").(*SignIn)
	next := ctx.TestScenes(s)
	s.signIn(ctx, link.SignIn{Code: "ABCD-EFGH"})
	<-in
	esc = 1
	if err := s.Update(ctx); err != nil {
		t.Fatal(err)
	}
	esc = 0
	if next() != nil {
		t.Fatal("Esc left the screen")
	}
	if !s.linking || s.step != siWaiting {
		t.Fatalf("Esc changed the screen: linking %v step %d", s.linking, s.step)
	}
	close(gate)
	deadline := time.Now().Add(5 * time.Second)
	for s.linking && time.Now().Before(deadline) {
		s.Update(ctx)
		time.Sleep(2 * time.Millisecond)
	}
	if s.linking || !ctx.Link.Linked() {
		t.Fatalf("sign-in did not finish: linking %v, %+v", s.linking, ctx.Link.Status())
	}
	if next() == nil {
		t.Fatal("the screen did not move on after signing in")
	}
}

// syncServer links, and answers /me from a gate it can hold; everything
// else is an empty success, so a sync can end well.
type syncServer struct {
	mu    sync.Mutex
	gates []chan struct{} // one per /me call, in order; nil: no wait
	meIn  chan int
	calls int
	fail  bool
}

func (f *syncServer) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/api/v1/link":
		json.NewEncoder(w).Encode(map[string]any{"device_id": "dev_1", "token_type": "Bearer",
			"access_token": "hwd_1", "expires_in": 86400, "refresh_token": "hwr_1", "refresh_expires_in": 86400})
	case "/api/v1/me":
		f.mu.Lock()
		n := f.calls
		f.calls++
		var g chan struct{}
		if n < len(f.gates) {
			g = f.gates[n]
		}
		fail := f.fail
		f.mu.Unlock()
		if f.meIn != nil {
			f.meIn <- n
		}
		if g != nil {
			<-g
		}
		if fail {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "invalid_request", "message": "no"}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"learner": map[string]any{"id": "lrn_1", "display_name": "Sam"}})
	default:
		json.NewEncoder(w).Encode(map[string]any{})
	}
}

func accountSyncCtx(t *testing.T, f *syncServer) (*Account, *game.Context) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	ctx := testContext(t)
	ctx.Link = link.Open(link.Options{Store: &lockedFiles{m: memFiles{}}, Server: srv.URL})
	if err := ctx.Link.LinkNow(context.Background(), "ABCD-EFGH"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ctx.Link.Close)
	return NewAccount(ctx).(*Account), ctx
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out: %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// Sync Now on an idle game ends with "Synced." when it worked.
func TestAccountSyncSaysSynced(t *testing.T) {
	f := &syncServer{}
	a, ctx := accountSyncCtx(t, f)
	enter := 0
	input.FakeKeys(t, func(k ebiten.Key) int {
		if k == ebiten.KeyEnter {
			return enter
		}
		return 0
	})
	enter = 1
	a.Update(ctx)
	enter = 0
	if a.msg != "Syncing…" || a.syncUntil != 1 {
		t.Fatalf("after pressing: %q, until %d", a.msg, a.syncUntil)
	}
	waitFor(t, "the sync", func() bool { s := ctx.Link.Status(); return s.Syncs >= 1 && !s.Busy })
	a.Update(ctx)
	if a.msg != "Synced." || a.syncUntil != 0 {
		t.Fatalf("after the sync: %q, until %d, status %+v", a.msg, a.syncUntil, ctx.Link.Status())
	}
}

// A sync already running when Sync Now is pressed does not count as the
// one asked for: the message waits for the next one to end.
func TestAccountSyncWhileSyncing(t *testing.T) {
	first := make(chan struct{})
	f := &syncServer{gates: []chan struct{}{first}, meIn: make(chan int, 4)}
	a, ctx := accountSyncCtx(t, f)
	enter := 0
	input.FakeKeys(t, func(k ebiten.Key) int {
		if k == ebiten.KeyEnter {
			return enter
		}
		return 0
	})
	ctx.Link.SyncNow() // the sync already running
	<-f.meIn
	waitFor(t, "busy", func() bool { return ctx.Link.Status().Busy })
	enter = 1
	a.Update(ctx)
	enter = 0
	if a.syncUntil != 2 || a.msg != "Syncing…" {
		t.Fatalf("until %d, msg %q", a.syncUntil, a.msg)
	}
	close(first)
	waitFor(t, "first sync", func() bool { return ctx.Link.Status().Syncs >= 1 })
	// Whatever the first one did, the screen still says Syncing.
	a.Update(ctx)
	if s := ctx.Link.Status(); s.Syncs == 1 && a.msg != "Syncing…" {
		t.Fatalf("message after only the running sync: %q", a.msg)
	}
	waitFor(t, "second sync", func() bool { s := ctx.Link.Status(); return s.Syncs >= 2 && !s.Busy })
	a.Update(ctx)
	if a.msg != "Synced." || a.syncUntil != 0 {
		t.Fatalf("after both: %q, until %d", a.msg, a.syncUntil)
	}
}

// A failed sync shows why, and an unlinked game clears the message.
func TestAccountSyncFailedAndUnlinked(t *testing.T) {
	f := &syncServer{fail: true}
	a, ctx := accountSyncCtx(t, f)
	a.say("Syncing…", pal.Ice)
	a.syncUntil = ctx.Link.Status().Syncs + 1
	ctx.Link.SyncNow()
	waitFor(t, "the sync", func() bool { s := ctx.Link.Status(); return s.Syncs >= 1 && !s.Busy })
	a.Update(ctx)
	if a.msg == "" || a.msg == "Syncing…" || a.msg == "Synced." || a.syncUntil != 0 {
		t.Fatalf("after a failed sync: %q", a.msg)
	}
	a.say("Syncing…", pal.Ice)
	a.syncUntil = 99
	ctx.Link.Unlink()
	a.Update(ctx)
	if a.msg != "" || a.syncUntil != 0 {
		t.Fatalf("after an unlink: %q, until %d", a.msg, a.syncUntil)
	}
}

// lockedFiles is memFiles that background syncs and the test can share.
type lockedFiles struct {
	mu sync.Mutex
	m  memFiles
}

func (l *lockedFiles) Read(n string) ([]byte, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.m.Read(n)
}
func (l *lockedFiles) Write(n string, d []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.m.Write(n, d)
}
func (l *lockedFiles) WritePrivate(n string, d []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.m.WritePrivate(n, d)
}
func (l *lockedFiles) Remove(n string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.m.Remove(n)
}
