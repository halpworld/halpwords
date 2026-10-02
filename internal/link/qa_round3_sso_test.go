package link

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// The verifier is sent on every poll: the pending ones, a slow_down one,
// one after a reload, and one whose answer is a 429.
func TestQASSOVerifierOnEveryPoll(t *testing.T) {
	f, c, st, _ := ssoSetup(t)
	f.pkce = true
	ctx := context.Background()
	if _, err := c.StartSSO(ctx, ""); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := c.PollSSO(ctx); !errors.Is(err, ErrSSOPending) {
			t.Fatal(err)
		}
	}
	f.slowDown = true
	c.PollSSO(ctx)
	f.slowDown = false
	// A reload between polls, twice: the verifier survives each.
	for i := 0; i < 2; i++ {
		c = Open(Options{Store: st, Server: c.server, Now: c.now})
		c.sleep = func(time.Duration) {}
		if c.PendingSSO() == nil {
			t.Fatal("the code was lost on reload")
		}
		if err := c.PollSSO(ctx); !errors.Is(err, ErrSSOPending) {
			t.Fatal(err)
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pollBodies) != 6 {
		t.Fatalf("%d polls", len(f.pollBodies))
	}
	first, _ := f.pollBodies[0]["code_verifier"].(string)
	for i, b := range f.pollBodies {
		if v, _ := b["code_verifier"].(string); v == "" || v != first || s256(v) != f.challenge {
			t.Errorf("poll %d sent verifier %q (first %q)", i, v, first)
		}
	}
}

// The verifier is in nothing the game hands out: not the StartSSO result,
// PendingSSO, Status, Peek, nor their JSON or %+v; a code that is dropped
// takes it out of link.json too.
func TestQASSOVerifierNeverHandedOut(t *testing.T) {
	f, c, st, _ := ssoSetup(t)
	f.pkce = true
	ctx := context.Background()
	got, err := c.StartSSO(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	c.PollSSO(ctx)
	f.mu.Lock()
	v, _ := f.pollBodies[0]["code_verifier"].(string)
	f.mu.Unlock()
	if len(v) != 43 {
		t.Fatalf("verifier %q", v)
	}
	pend := c.PendingSSO()
	// A caller changing its copy must not reach the stored code.
	got.Every, pend.Every = time.Hour, time.Hour
	if again := c.PendingSSO(); again.Every == time.Hour {
		t.Error("PendingSSO shares memory with the stored code")
	}
	all := []any{got, pend, c.Status(), PeekFolder(st)}
	for i, x := range all {
		j, _ := json.Marshal(x)
		if s := fmt.Sprintf("%+v|%#v|%s", x, x, j); strings.Contains(s, v) || strings.Contains(s, "code_verifier") {
			t.Errorf("handed-out value %d (%T) shows the verifier: %s", i, x, s)
		}
	}
	// Still polls with it after the copies were changed.
	c.PollSSO(ctx)
	f.mu.Lock()
	if v2, _ := f.pollBodies[1]["code_verifier"].(string); v2 != v {
		t.Errorf("second poll sent %q", v2)
	}
	f.mu.Unlock()
	// Dropping the code (Esc) wipes it from the file.
	c.CancelSSO()
	if raw, _ := st.Read(stateFile); strings.Contains(string(raw), v) {
		t.Error("the verifier stayed in link.json after Esc")
	}
}

// A start whose code is refused or expired leaves no verifier behind.
func TestQASSOExpiredCodeWipesVerifier(t *testing.T) {
	f, c, st, clk := ssoSetup(t)
	ctx := context.Background()
	c.StartSSO(ctx, "")
	c.PollSSO(ctx)
	f.mu.Lock()
	v, _ := f.pollBodies[0]["code_verifier"].(string)
	f.mu.Unlock()
	clk.mu.Lock()
	clk.t = clk.t.Add(time.Hour)
	clk.mu.Unlock()
	if err := c.PollSSO(ctx); !errors.Is(err, ErrSSOExpired) {
		t.Fatal(err)
	}
	if raw, _ := st.Read(stateFile); strings.Contains(string(raw), v) {
		t.Error("the verifier stayed in link.json after the code expired")
	}
}

// A rate limit on start or poll, with Retry-After missing, short, long
// (>= 60), exactly 60, zero, negative, a date, or garbage.
func TestQASSORateLimitRetryAfterVariants(t *testing.T) {
	cases := []struct {
		header string
		want   string
		after  time.Duration
	}{
		{"", "too many tries: wait a moment and try again", 0},
		{"1", "too many tries: wait 1 second and try again", time.Second},
		{"59", "too many tries: wait 59 seconds and try again", 59 * time.Second},
		{"60", "too many tries: wait a moment and try again", 60 * time.Second},
		{"120", "too many tries: wait a moment and try again", 120 * time.Second},
		{"86400", "too many tries: wait a moment and try again", 86400 * time.Second},
		{"0", "too many tries: wait a moment and try again", 0},
		{"-5", "too many tries: wait a moment and try again", 0},
		{"soon", "too many tries: wait a moment and try again", 0},
		{"Wed, 21 Oct 2026 07:28:00 GMT", "too many tries: wait a moment and try again", 0},
		{"1.5", "too many tries: wait a moment and try again", 0},
		{"99999999999999999999", "too many tries: wait a moment and try again", 0},
	}
	for _, body := range []string{"rate_limited", "", "something_else"} {
		for _, path := range []string{"start", "poll"} {
			for _, tc := range cases {
				name := fmt.Sprintf("%s/code=%q/header=%q", path, body, tc.header)
				t.Run(name, func(t *testing.T) {
					var mu sync.Mutex
					srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						mu.Lock()
						defer mu.Unlock()
						if r.URL.Path == "/api/v1/sso/start" && path == "poll" {
							writeJSON(w, 200, map[string]any{"device_code": "hwsso_abc", "user_code": "ABCD-EFGH",
								"verification_uri": "https://h.test/d", "expires_in": 600, "interval": 5})
							return
						}
						if tc.header != "" {
							w.Header().Set("Retry-After", tc.header)
						}
						if body == "" {
							w.WriteHeader(429)
							w.Write([]byte("slow down"))
							return
						}
						writeErr(w, 429, body)
					}))
					t.Cleanup(srv.Close)
					clk := &clock{t: time.Date(2026, 9, 26, 14, 5, 9, 0, time.UTC)}
					c := Open(Options{Store: newMemStore(), Server: srv.URL, Now: clk.now})
					c.sleep = func(time.Duration) {}
					ctx := context.Background()
					var err error
					if path == "start" {
						_, err = c.StartSSO(ctx, "")
					} else {
						if _, e := c.StartSSO(ctx, ""); e != nil {
							t.Fatal(e)
						}
						err = c.PollSSO(ctx)
					}
					var e *Error
					if !errors.Is(err, ErrSSOBusy) || !errors.As(err, &e) || e.RetryAfter != tc.after {
						t.Fatalf("err %v, RetryAfter %v", err, e)
					}
					if got := Explain(err); got != tc.want {
						t.Errorf("Explain %q, want %q", got, tc.want)
					}
					if path == "poll" && c.PendingSSO() == nil {
						t.Error("a rate-limited poll dropped the code")
					}
					if path == "start" && c.PendingSSO() != nil {
						t.Error("a failed start left a code")
					}
				})
			}
		}
	}
}

// The remote unlink text reaches the player through Explain and the note,
// and none of it blames the website.
func TestQARemoteUnlinkText(t *testing.T) {
	if got := Explain(ErrUnlinked); got != "this game was unlinked" {
		t.Errorf("Explain(ErrUnlinked) %q", got)
	}
	wrapped := fmt.Errorf("sync: %w", ErrUnlinked)
	if got := Explain(wrapped); strings.Contains(strings.ToLower(got), "website") || !strings.Contains(got, "unlinked") {
		t.Errorf("Explain(wrapped) %q", got)
	}
	// A 401 on refresh unlinks the game with the neutral note, once.
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		writeErr(w, http.StatusUnauthorized, "invalid_token")
	}))
	t.Cleanup(srv.Close)
	c := Open(Options{Store: newMemStore(), Server: srv.URL})
	t.Cleanup(c.Close)
	c.sleep = func(time.Duration) {}
	c.mu.Lock()
	c.signedIn(tokens{AccessToken: "a", RefreshToken: "r", ExpiresIn: 3600, RefreshExpiresIn: 3600}, WayPairing)
	gen := c.gen
	c.mu.Unlock()
	c.lost(gen)
	st := c.Status()
	if st.Note != "This game was unlinked. Your progress is still here." || c.Linked() {
		t.Fatalf("note %q linked %v", st.Note, c.Linked())
	}
	// A stale generation does not repeat or overwrite it.
	c.mu.Lock()
	c.note = ""
	c.mu.Unlock()
	c.lost(gen)
	if n := c.Status().Note; n != "" {
		t.Errorf("a stale lost() wrote a note: %q", n)
	}
}

// A slow /sso/token answer after Esc (CancelSSO) is revoked and does not
// link, also when the poll was the second one and when the revoke itself
// finds an expired access token.
func TestQASSOSlowAnswerAfterEscRevoked(t *testing.T) {
	f, c, _, _ := ssoSetup(t)
	ctx := context.Background()
	c.StartSSO(ctx, "")
	f.signedIn = true
	f.gate, f.entered = make(chan struct{}), make(chan struct{}, 2)
	done := make(chan error, 1)
	go func() { done <- c.PollSSO(ctx) }()
	<-f.entered
	// Esc pressed fast, three times.
	c.CancelSSO()
	c.CancelSSO()
	c.CancelSSO()
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
	if c.PendingSSO() != nil {
		t.Error("a code is pending after Esc")
	}
}

// CancelSignIn pressed repeatedly: the first cancels, the rest say no,
// and a new SignIn after it still works and is not cancelled by the old
// one's late answer.
func TestQACancelSignInTwiceThenAgain(t *testing.T) {
	var mu sync.Mutex
	var unlinks []string
	n := 0
	gates := []chan struct{}{make(chan struct{}), make(chan struct{})}
	entered := make(chan int, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/link":
			mu.Lock()
			i := n
			n++
			mu.Unlock()
			entered <- i
			<-gates[i]
			writeJSON(w, 200, map[string]any{"device_id": "dev_1", "token_type": "Bearer", "access_token": fmt.Sprintf("hwd_%d", i),
				"expires_in": 3600, "refresh_token": fmt.Sprintf("hwr_%d", i), "refresh_expires_in": 86400})
		case "/api/v1/unlink":
			mu.Lock()
			unlinks = append(unlinks, r.Header.Get("Authorization"))
			mu.Unlock()
			w.WriteHeader(204)
		default:
			w.WriteHeader(503)
		}
	}))
	t.Cleanup(srv.Close)
	c := Open(Options{Store: newMemStore(), Server: srv.URL})
	t.Cleanup(c.Close)
	c.SignIn(SignIn{Code: "ABCD-EFGH-JKLM"})
	<-entered
	if !c.CancelSignIn() {
		t.Fatal("first cancel")
	}
	for i := 0; i < 3; i++ {
		if c.CancelSignIn() {
			t.Fatal("a repeated cancel said yes")
		}
	}
	// Second sign-in starts while the first is still held on the server
	// (syncMu serialises them: release the first answer first).
	c.SignIn(SignIn{Code: "ABCD-EFGH-JKLM"})
	close(gates[0])
	<-entered
	if !c.Status().Signing {
		t.Fatal("second sign-in not shown as signing")
	}
	close(gates[1])
	deadline := time.Now().Add(5 * time.Second)
	for !c.Linked() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	c.bg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if !c.Linked() {
		t.Fatalf("the second sign-in did not link: %+v", c.Status())
	}
	if len(unlinks) != 1 || unlinks[0] != "Bearer hwd_0" {
		t.Fatalf("unlinks %q", unlinks)
	}
}
