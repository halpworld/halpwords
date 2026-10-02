package scene

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/browser"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/link"
)

// slowServer answers the sign-in requests only when release is closed, and
// notes the unlinks it is told.
type slowServer struct {
	mu      sync.Mutex
	release chan struct{}
	entered chan string
	unlinks []string
}

func newSlowServer(t *testing.T) (*slowServer, *httptest.Server) {
	t.Helper()
	f := &slowServer{release: make(chan struct{}), entered: make(chan string, 8)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/unlink" {
			f.mu.Lock()
			f.unlinks = append(f.unlinks, r.Header.Get("Authorization"))
			f.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		f.entered <- r.URL.Path
		<-f.release
		switch r.URL.Path {
		case "/api/v1/sso/start":
			json.NewEncoder(w).Encode(map[string]any{"device_code": "hwsso_abc", "user_code": "ABCD-EFGH",
				"verification_uri": "https://halpwords.test/sso/device", "expires_in": 600, "interval": 5})
		case "/api/v1/link":
			json.NewEncoder(w).Encode(map[string]any{"device_id": "dev_1", "token_type": "Bearer", "access_token": "hwd_late",
				"expires_in": 3600, "refresh_token": "hwr_late", "refresh_expires_in": 86400})
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

// open lets the held requests answer, once.
func (f *slowServer) open() {
	f.mu.Lock()
	defer f.mu.Unlock()
	select {
	case <-f.release:
	default:
		close(f.release)
	}
}

func signInScreen(t *testing.T, f *slowServer, srv *httptest.Server) (*SignIn, *game.Context) {
	t.Helper()
	openPage = func(string) error { return nil }
	t.Cleanup(func() { openPage = browser.Open })
	ctx := testContext(t)
	ctx.Input = &input.State{}
	ctx.Link = link.Open(link.Options{Store: &lockedFiles{m: memFiles{}}, Server: srv.URL})
	t.Cleanup(ctx.Link.Close)
	t.Cleanup(f.open) // before the link closes, which waits for its requests
	return NewSignIn(ctx, "").(*SignIn), ctx
}

func noKeys(t *testing.T) { t.Helper(); holdKey(t, ebiten.KeyF24) }

// Esc while the game waits for a school sign-in code stops the wait; a code
// that comes late is forgotten (#67).
func TestSignInEscWhileStartingSSO(t *testing.T) {
	f, srv := newSlowServer(t)
	s, ctx := signInScreen(t, f, srv)
	s.startSSO(ctx)
	<-f.entered
	press(t, ebiten.KeyEscape)
	s.Update(ctx)
	if s.pending != nil || s.step != siChoose {
		t.Fatalf("Esc did nothing: pending %v, step %d", s.pending != nil, s.step)
	}
	noKeys(t)
	f.open()
	time.Sleep(100 * time.Millisecond)
	s.Update(ctx)
	if s.step != siChoose || s.sso != nil || ctx.Link.PendingSSO() != nil {
		t.Fatalf("a late code was taken: step %d, pending %v", s.step, ctx.Link.PendingSSO())
	}
}

// Esc while the class code is being looked up goes back a step.
func TestSignInEscWhileAskingForClass(t *testing.T) {
	f, srv := newSlowServer(t)
	s, ctx := signInScreen(t, f, srv)
	s.step, s.classCode = siClass, "C1A55C0D"
	s.ask(ctx, "", "")
	<-f.entered
	press(t, ebiten.KeyEscape)
	s.Update(ctx)
	if s.pending != nil || s.step != siChoose {
		t.Fatalf("Esc did nothing: pending %v, step %d", s.pending != nil, s.step)
	}
}

// Esc while "Signing in…" waits goes back to the code, and the tokens that
// arrive late are dropped and revoked on the server (#67, #69).
func TestSignInEscWhileSigningIn(t *testing.T) {
	f, srv := newSlowServer(t)
	s, ctx := signInScreen(t, f, srv)
	s.step = siPairing
	s.signIn(ctx, link.SignIn{Code: "ABCD-EFGH-JKLM"})
	<-f.entered
	if !s.linking || s.step != siWaiting {
		t.Fatal("not waiting")
	}
	press(t, ebiten.KeyEscape)
	s.Update(ctx)
	if s.linking || s.step != siPairing || ctx.Link.Status().Signing {
		t.Fatalf("Esc did nothing: linking %v, step %d", s.linking, s.step)
	}
	noKeys(t)
	f.open()
	deadline := time.Now().Add(5 * time.Second)
	for {
		f.mu.Lock()
		n := len(f.unlinks)
		f.mu.Unlock()
		if n > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.Update(ctx)
	f.mu.Lock()
	defer f.mu.Unlock()
	if ctx.Link.Linked() || len(f.unlinks) != 1 || f.unlinks[0] != "Bearer hwd_late" || s.step != siPairing {
		t.Fatalf("linked %v, unlinks %q, step %d", ctx.Link.Linked(), f.unlinks, s.step)
	}
}
