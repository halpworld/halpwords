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
	used     bool
	name     string
}

func (f *ssoServer) serve(w http.ResponseWriter, r *http.Request) {
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
