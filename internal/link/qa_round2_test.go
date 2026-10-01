package link

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// pollServer answers every /sso/token poll with one fixed error.
func pollServer(t *testing.T, status int, code string, retryAfter string) (*Client, *memStore, *clock) {
	t.Helper()
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/api/v1/sso/start" {
			writeJSON(w, http.StatusOK, map[string]any{"device_code": "hwsso_abc", "user_code": "ABCD-EFGH",
				"verification_uri": "https://halpwords.test/sso/device", "expires_in": 600, "interval": 5})
			return
		}
		if retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		writeErr(w, status, code)
	}))
	t.Cleanup(srv.Close)
	st := newMemStore()
	clk := &clock{t: time.Date(2026, 9, 26, 14, 5, 9, 0, time.UTC)}
	c := Open(Options{Store: st, Server: srv.URL, Now: clk.now})
	c.sleep = func(time.Duration) {}
	if _, err := c.StartSSO(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	return c, st, clk
}

// A poll the server answers with a rate limit or an error of its own
// keeps the code; one that says the sign-in is over drops it.
func TestSSOPollErrorsKeepOrDropTheCode(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		code       string
		retryAfter string
		want       error // nil: any error that is not one of the sso ones
		keep       bool
	}{
		{"rate limited", 429, "rate_limited", "3", nil, true},
		{"429 without code", 429, "", "", nil, true},
		{"rate limited for too long", 429, "rate_limited", "120", nil, false},
		{"server error", 503, "unavailable", "", nil, true},
		{"internal error", 500, "internal", "", nil, true},
		{"slow_down", 400, codeSlowDown, "", ErrSSOPending, true},
		{"pending", 400, codeAuthorizationPending, "", ErrSSOPending, true},
		{"expired_token", 400, codeExpiredToken, "", ErrSSOExpired, false},
		{"access_denied", 403, codeAccessDenied, "", ErrSSORefused, false},
		{"other 4xx", 400, "invalid_request", "", nil, false},
		{"not found", 404, "not_found", "", nil, false},
		{"unauthorized", 401, "invalid_token", "", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _, _ := pollServer(t, tc.status, tc.code, tc.retryAfter)
			err := c.PollSSO(context.Background())
			if err == nil {
				t.Fatal("no error")
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if tc.want == nil && (errors.Is(err, ErrSSOPending) || errors.Is(err, ErrSSOExpired) || errors.Is(err, ErrSSORefused)) {
				t.Fatalf("an error the server never named was explained as %v", err)
			}
			if kept := c.PendingSSO() != nil; kept != tc.keep {
				t.Fatalf("code kept = %v, want %v (err %v)", kept, tc.keep, err)
			}
			if c.Linked() {
				t.Fatal("linked")
			}
		})
	}
}

// A server error after the code has run out says the code ran out.
func TestSSOPollServerErrorAfterExpiry(t *testing.T) {
	c, st, clk := pollServer(t, 503, "unavailable", "")
	clk.add(11 * time.Minute)
	if err := c.PollSSO(context.Background()); !errors.Is(err, ErrSSOExpired) {
		t.Fatalf("got %v", err)
	}
	if c.PendingSSO() != nil {
		t.Fatal("expired code kept")
	}
	if c2 := Open(Options{Store: st, Server: c.server, Now: clk.now}); c2.PendingSSO() != nil {
		t.Fatal("expired code survived a reload")
	}
}

// slow_down adds up across polls and reloads: a game that reloads and
// polls again goes on from the slower interval.
func TestSSOSlowDownKeepsAddingAfterReload(t *testing.T) {
	f, c, st, _ := ssoSetup(t)
	ctx := context.Background()
	if _, err := c.StartSSO(ctx, ""); err != nil {
		t.Fatal(err)
	}
	f.slowDown = true
	c.PollSSO(ctx)
	c2 := Open(Options{Store: st, Server: c.server, Now: c.now})
	c2.sleep = func(time.Duration) {}
	if p := c2.PendingSSO(); p == nil || p.Every != 10*time.Second {
		t.Fatalf("after reload %+v", p)
	}
	if err := c2.PollSSO(ctx); !errors.Is(err, ErrSSOPending) {
		t.Fatal(err)
	}
	if p := c2.PendingSSO(); p.Every != 15*time.Second {
		t.Fatalf("after another slow_down %v", p.Every)
	}
	c3 := Open(Options{Store: st, Server: c.server, Now: c.now})
	if p := c3.PendingSSO(); p == nil || p.Every != 15*time.Second {
		t.Fatalf("after a second reload %+v", p)
	}
}

// A slow_down that arrives after the pupil pressed Esc must not bring
// the cancelled code back.
func TestSSOSlowDownAfterCancelDoesNotResurrect(t *testing.T) {
	f, c, st, _ := ssoSetup(t)
	ctx := context.Background()
	if _, err := c.StartSSO(ctx, ""); err != nil {
		t.Fatal(err)
	}
	f.slowDown = true
	f.gate, f.entered = make(chan struct{}), make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() { done <- c.PollSSO(ctx) }()
	<-f.entered
	c.CancelSSO()
	close(f.gate)
	<-done
	if c.PendingSSO() != nil {
		t.Fatal("slow_down brought the cancelled code back")
	}
	if c2 := Open(Options{Store: st, Server: c.server, Now: c.now}); c2.PendingSSO() != nil {
		t.Fatal("cancelled code on disk")
	}
}

// Signing is true while a sign-in waits and false after every way it can
// end, and Busy goes with it.
func TestSigningResetsOnEveryEnd(t *testing.T) {
	ctx := context.Background()
	check := func(t *testing.T, c *Client, wantErr func(error) bool, err error) {
		t.Helper()
		if !wantErr(err) {
			t.Fatalf("err %v", err)
		}
		if st := c.Status(); st.Signing || st.Busy {
			t.Fatalf("still signing or busy: %+v", st)
		}
	}
	t.Run("wrong code", func(t *testing.T) {
		_, c, _, _ := setup(t)
		check(t, c, func(e error) bool { return errors.Is(e, ErrBadCode) }, c.LinkNow(ctx, "WXYZ-WXYZ"))
	})
	t.Run("malformed code", func(t *testing.T) {
		_, c, _, _ := setup(t)
		check(t, c, func(e error) bool { return errors.Is(e, ErrBadCode) }, c.LinkNow(ctx, "abc"))
	})
	t.Run("server error", func(t *testing.T) {
		f, c, _, _ := setup(t)
		f.failNext("/api/v1/link", 503)
		check(t, c, func(e error) bool { return e != nil }, c.LinkNow(ctx, "abcd-efgh"))
	})
	t.Run("cancelled context", func(t *testing.T) {
		_, c, _, _ := setup(t)
		cc, cancel := context.WithCancel(ctx)
		cancel()
		check(t, c, func(e error) bool { return e != nil }, c.LinkNow(cc, "abcd-efgh"))
	})
	t.Run("already linked", func(t *testing.T) {
		_, c, _, _ := linked(t)
		check(t, c, func(e error) bool { return errors.Is(e, ErrLinked) }, c.LinkNow(ctx, "abcd-efgh"))
	})
	t.Run("success", func(t *testing.T) {
		_, c, _, _ := setup(t)
		check(t, c, func(e error) bool { return e == nil }, c.LinkNow(ctx, "abcd-efgh"))
	})
	t.Run("signing while the request is on its way", func(t *testing.T) {
		f := newFake(t)
		gate, in := make(chan struct{}), make(chan struct{}, 1)
		slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/link" {
				in <- struct{}{}
				<-gate
			}
			f.srv.Config.Handler.ServeHTTP(w, r)
		}))
		t.Cleanup(slow.Close)
		c := Open(Options{Store: newMemStore(), Server: slow.URL, OwnDir: "words"})
		c.sleep = func(time.Duration) {}
		done := make(chan error, 1)
		go func() { done <- c.LinkNow(ctx, "abcd-efgh") }()
		<-in
		if st := c.Status(); !st.Signing || !st.Busy {
			t.Fatalf("not signing: %+v", st)
		}
		close(gate)
		check(t, c, func(e error) bool { return e == nil }, <-done)
	})
}

// A game never in a room that is unlinked from the website has no room
// to leave: lost must not trip over that.
func TestLostWithoutARoom(t *testing.T) {
	f, c, _, _ := linked(t)
	if c.play != nil {
		t.Fatal("play already used")
	}
	loseLink(t, f, c)
	if c.Linked() {
		t.Fatal("still linked")
	}
	if st := c.Play().State(); st.Phase != PlayOff {
		t.Fatalf("phase %v", st.Phase)
	}
	// A second loss for the same, already gone, link does nothing.
	c.lost(c.gen)
}
