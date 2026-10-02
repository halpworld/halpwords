package link

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
	interval int64 // the interval start answers; 0 is 5
	slowDown bool
	gate     chan struct{}
	entered  chan struct{}
	// pkce makes the server bind a code started with a code_challenge to
	// its code_verifier, as halpwords-server#98 does; without it the
	// server is an older one that ignores both fields.
	pkce       bool
	challenge  string
	pollBodies []map[string]any
	// unlinks are the Authorization headers of POST /api/v1/unlink.
	unlinks []string
	// rateLimit answers sso/start with 429 rate_limited and Retry-After 7.
	rateLimit bool
}

func s256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
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
	case "/api/v1/unlink":
		f.unlinks = append(f.unlinks, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusNoContent)
	case "/api/v1/sso/start":
		if f.rateLimit {
			w.Header().Set("Retry-After", "7")
			writeErr(w, http.StatusTooManyRequests, codeRateLimited)
			return
		}
		f.started = body
		f.challenge, _ = body["code_challenge"].(string)
		writeJSON(w, http.StatusOK, map[string]any{"device_code": "hwsso_abc", "user_code": "ABCD-EFGH",
			"verification_uri": "https://halpwords.test/sso/device", "verification_uri_complete": "https://halpwords.test/sso/device?code=ABCD-EFGH",
			"expires_in": 600, "interval": cmp.Or(f.interval, 5)})
	case "/api/v1/sso/token":
		f.polls++
		f.pollBodies = append(f.pollBodies, body)
		verifier, _ := body["code_verifier"].(string)
		switch {
		case body["device_code"] != "hwsso_abc" || f.used,
			f.pkce && f.challenge != "" && s256(verifier) != f.challenge:
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

// An old poll that comes back after Esc and a new code must not wipe the
// new code, whichever way it ends.
func TestSSOStalePollDoesNotWipeNewCode(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(f *ssoServer)
	}{
		{"expired", func(f *ssoServer) { f.used = true }},
		{"refused", func(f *ssoServer) { f.refused = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, c, _, _ := ssoSetup(t)
			ctx := context.Background()
			if _, err := c.StartSSO(ctx, ""); err != nil {
				t.Fatal(err)
			}
			tc.setup(f)
			f.gate, f.entered = make(chan struct{}), make(chan struct{}, 1)
			done := make(chan error, 1)
			go func() { done <- c.PollSSO(ctx) }()
			<-f.entered
			c.CancelSSO()
			again, err := c.StartSSO(ctx, "")
			if err != nil {
				t.Fatal(err)
			}
			close(f.gate)
			<-done
			if p := c.PendingSSO(); p == nil || p.DeviceCode != again.DeviceCode {
				t.Fatalf("the new code was wiped: %+v", p)
			}
		})
	}
}

// However often the server says slow_down, or whatever interval it
// starts with, the game polls at least once a minute.
func TestSSOEveryIsCapped(t *testing.T) {
	f, c, _, _ := ssoSetup(t)
	ctx := context.Background()
	f.interval = 300
	code, err := c.StartSSO(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if code.Every != time.Minute {
		t.Fatalf("started at %v", code.Every)
	}
	f.interval = 5
	if _, err := c.StartSSO(ctx, ""); err != nil {
		t.Fatal(err)
	}
	f.slowDown = true
	for range 20 {
		c.PollSSO(ctx)
	}
	if p := c.PendingSSO(); p == nil || p.Every != time.Minute {
		t.Fatalf("after many slow_downs: %+v", p)
	}
}

// The game binds its code with PKCE (halpwords-server#98): start sends the
// S256 challenge of a random 32-byte verifier, and every poll sends the
// verifier, also after a reload. An older server ignores both fields.
func TestSSOBindsCodeWithPKCE(t *testing.T) {
	for _, pkce := range []bool{true, false} {
		name := map[bool]string{true: "server with PKCE", false: "older server"}[pkce]
		t.Run(name, func(t *testing.T) {
			f, c, st, _ := ssoSetup(t)
			f.pkce = pkce
			ctx := context.Background()
			code, err := c.StartSSO(ctx, "")
			if err != nil {
				t.Fatal(err)
			}
			ch := f.challenge
			if len(ch) != 43 || strings.ContainsAny(ch, "=+/") {
				t.Fatalf("code_challenge %q is not 43 unpadded base64url characters", ch)
			}
			if f.started["code_challenge_method"] != nil && f.started["code_challenge_method"] != "S256" {
				t.Errorf("method %v", f.started["code_challenge_method"])
			}
			if code.Verifier != "" || c.PendingSSO().Verifier != "" {
				t.Error("the screens were handed the verifier")
			}
			if err := c.PollSSO(ctx); !errors.Is(err, ErrSSOPending) {
				t.Fatal(err)
			}
			v, _ := f.pollBodies[0]["code_verifier"].(string)
			if len(v) != 43 || s256(v) != ch {
				t.Fatalf("poll sent verifier %q for challenge %q", v, ch)
			}
			raw, _ := st.Read(stateFile)
			if !strings.Contains(string(raw), v) {
				t.Error("the verifier isn't in link.json: a reload couldn't go on")
			}
			// After a reload the new page polls with the same verifier.
			c2 := Open(Options{Store: st, Server: c.server, Now: c.now})
			f.signedIn = true
			if err := c2.PollSSO(ctx); err != nil || !c2.Linked() {
				t.Fatalf("after a reload: %v", err)
			}
			if raw, _ := st.Read(stateFile); strings.Contains(string(raw), v) {
				t.Error("the verifier stayed in link.json after signing in")
			}
		})
	}
}

// Each start makes a new verifier.
func TestSSOVerifierIsFresh(t *testing.T) {
	f, c, _, _ := ssoSetup(t)
	ctx := context.Background()
	c.StartSSO(ctx, "")
	a := f.challenge
	c.StartSSO(ctx, "")
	if a == "" || a == f.challenge {
		t.Fatalf("challenges %q then %q", a, f.challenge)
	}
}

// A code with a wrong verifier answers expired_token: the game starts again.
func TestSSOWrongVerifierMeansStartAgain(t *testing.T) {
	f, c, _, _ := ssoSetup(t)
	f.pkce, f.signedIn = true, true
	ctx := context.Background()
	if _, err := c.StartSSO(ctx, ""); err != nil {
		t.Fatal(err)
	}
	f.challenge = s256("some other verifier")
	if err := c.PollSSO(ctx); !errors.Is(err, ErrSSOExpired) || c.Linked() || c.PendingSSO() != nil {
		t.Fatalf("wrong verifier: %v", err)
	}
}

// The verifier is a secret of link.json, which is written private and
// never moves with the player (move.Exportable, tested in internal/move):
// it is in no other file.
func TestSSOVerifierStaysPrivate(t *testing.T) {
	f, c, st, _ := ssoSetup(t)
	f.pkce = true
	ctx := context.Background()
	if _, err := c.StartSSO(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := c.PollSSO(ctx); !errors.Is(err, ErrSSOPending) {
		t.Fatal(err)
	}
	v, _ := f.pollBodies[0]["code_verifier"].(string)
	st.mu.Lock()
	defer st.mu.Unlock()
	for name, data := range st.files {
		if strings.Contains(string(data), v) && (name != stateFile || !st.private[name]) {
			t.Errorf("the verifier is in %s (private %v)", name, st.private[name])
		}
	}
}

// Tokens that come back after Esc, or for a replaced code, are revoked on
// the server (POST /api/v1/unlink), best effort.
func TestSSODroppedTokensAreRevoked(t *testing.T) {
	for _, how := range []string{"cancelled", "replaced"} {
		t.Run(how, func(t *testing.T) {
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
			if how == "replaced" {
				if _, err := c.StartSSO(ctx, ""); err != nil {
					t.Fatal(err)
				}
			}
			close(f.gate)
			if err := <-done; !errors.Is(err, ErrSSOExpired) {
				t.Fatalf("poll: %v", err)
			}
			c.bg.Wait()
			f.mu.Lock()
			defer f.mu.Unlock()
			if c.Linked() || len(f.unlinks) != 1 || f.unlinks[0] != "Bearer hwd_1" {
				t.Fatalf("linked %v, unlinks %q", c.Linked(), f.unlinks)
			}
		})
	}
}

// A start that is cancelled keeps no code.
func TestSSOCancelledStartKeepsNothing(t *testing.T) {
	_, c, st, _ := ssoSetup(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.StartSSO(ctx, ""); err == nil || c.PendingSSO() != nil || st.has(stateFile) {
		t.Fatalf("cancelled start: %v", err)
	}
}

// A rate_limited answer is explained, with how long to wait.
func TestSSORateLimitedIsExplained(t *testing.T) {
	f, c, _, _ := ssoSetup(t)
	f.rateLimit = true
	_, err := c.StartSSO(context.Background(), "")
	var e *Error
	if !errors.Is(err, ErrSSOBusy) || !errors.As(err, &e) || e.RetryAfter != 7*time.Second {
		t.Fatalf("rate limited: %v", err)
	}
	if got := Explain(err); got != "too many tries: wait 7 seconds and try again" {
		t.Errorf("Explain: %q", got)
	}
	if got := Explain(explainSSO(&Error{Status: 429, Code: codeRateLimited})); got != ErrSSOBusy.Error() {
		t.Errorf("without Retry-After: %q", got)
	}
	// A poll that is rate limited keeps the code.
	f.rateLimit = false
	if _, err := c.StartSSO(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
}

// A sign-in the player gave up on (Esc) keeps nothing: tokens that still
// come back are revoked, and Status doesn't say Signing.
func TestCancelSignInRevokesLateTokens(t *testing.T) {
	var mu sync.Mutex
	var unlinks []string
	entered, gate := make(chan struct{}, 1), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/link":
			entered <- struct{}{}
			<-gate
			writeJSON(w, http.StatusOK, map[string]any{"device_id": "dev_1", "token_type": "Bearer", "access_token": "hwd_late",
				"expires_in": 3600, "refresh_token": "hwr_late", "refresh_expires_in": 86400})
		case "/api/v1/unlink":
			mu.Lock()
			unlinks = append(unlinks, r.Header.Get("Authorization"))
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(srv.Close)
	c := Open(Options{Store: newMemStore(), Server: srv.URL})
	t.Cleanup(c.Close)
	if c.CancelSignIn() {
		t.Fatal("cancelled a sign-in that wasn't on its way")
	}
	c.SignIn(SignIn{Code: "ABCD-EFGH-JKLM"})
	<-entered
	if !c.Status().Signing {
		t.Fatal("not signing in")
	}
	if !c.CancelSignIn() {
		t.Fatal("no sign-in to cancel")
	}
	if st := c.Status(); st.Signing || st.Err != nil {
		t.Fatalf("after cancelling: %+v", st)
	}
	close(gate)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(unlinks)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	c.bg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if c.Linked() || len(unlinks) != 1 || unlinks[0] != "Bearer hwd_late" {
		t.Fatalf("linked %v, unlinks %q", c.Linked(), unlinks)
	}
	if st := c.Status(); st.Err != nil || st.Signing {
		t.Fatalf("after the late answer: err %v signing %v", st.Err, st.Signing)
	}
}

// The note after a remote unlink doesn't say who did it: a family or a
// school can end the connection too.
func TestUnlinkNoteIsNeutral(t *testing.T) {
	c := Open(Options{Store: newMemStore(), Server: "http://127.0.0.1:1"})
	t.Cleanup(c.Close)
	c.mu.Lock()
	c.signedIn(tokens{AccessToken: "a", RefreshToken: "r", ExpiresIn: 60, RefreshExpiresIn: 60}, WayPairing)
	gen := c.gen
	c.mu.Unlock()
	c.lost(gen)
	if got := c.Status().Note; got != "This game was unlinked. Your progress is still here." {
		t.Errorf("note %q", got)
	}
	if got := ErrUnlinked.Error(); strings.Contains(got, "website") {
		t.Errorf("ErrUnlinked: %q", got)
	}
}
