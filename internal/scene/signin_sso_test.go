package scene

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/halpworld/halpwords/internal/browser"
	"github.com/halpworld/halpwords/internal/link"
)

// TestSignInWithSchoolAccount goes through signing in with a school
// account: the code, the page opened, waiting, and the sign-in.
func TestSignInWithSchoolAccount(t *testing.T) {
	var mu sync.Mutex
	signedIn := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/sso/start":
			json.NewEncoder(w).Encode(map[string]any{"device_code": "hwsso_abc", "user_code": "ABCD-EFGH",
				"verification_uri": "https://halpwords.test/sso/device", "verification_uri_complete": "https://halpwords.test/sso/device?code=ABCD-EFGH",
				"expires_in": 600, "interval": 5})
		case r.URL.Path == "/api/v1/sso/token" && !signedIn:
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "authorization_pending", "message": "wait"}})
		case r.URL.Path == "/api/v1/sso/token":
			json.NewEncoder(w).Encode(map[string]any{"device_id": "dev_1", "token_type": "Bearer",
				"access_token": "hwd_1", "expires_in": 86400, "refresh_token": "hwr_1", "refresh_expires_in": 86400})
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(srv.Close)
	var opened []string
	openPage = func(u string) error { opened = append(opened, u); return nil }
	t.Cleanup(func() { openPage = browser.Open })
	ctx := testContext(t)
	files := memFiles{}
	ctx.Link = link.Open(link.Options{Store: files, Server: srv.URL})
	t.Cleanup(ctx.Link.Close)

	s := NewSignIn(ctx, "").(*SignIn)
	if s.ways()[2] != siWaySSO {
		t.Fatalf("ways %v", s.ways())
	}
	s.startSSO(ctx)
	s.answer(ctx, <-s.pending)
	if s.step != siSSO || s.sso.UserCode != "ABCD-EFGH" || len(opened) != 1 || opened[0] != s.sso.VerifyURL {
		t.Fatalf("step %d, code %+v, opened %v", s.step, s.sso, opened)
	}
	// The game waits between polls.
	s.updateSSO(ctx)
	if s.polling != nil {
		t.Fatal("polled at once")
	}
	// A web game back from the website goes on where it was.
	if r, ok := resumeSSO(ctx).(*SignIn); !ok || r.step != siSSO || r.sso.DeviceCode != "hwsso_abc" {
		t.Fatal("no sign-in to resume")
	}
	ctx.Tick = s.nextPoll
	s.updateSSO(ctx)
	if s.polling == nil {
		t.Fatal("didn't poll")
	}
	s.polled(ctx, waitPoll(t, s))
	if s.step != siSSO || ctx.Link.Linked() {
		t.Fatalf("pending: step %d, %q", s.step, s.msg)
	}
	mu.Lock()
	signedIn = true
	mu.Unlock()
	ctx.Tick = s.nextPoll
	s.updateSSO(ctx)
	if err := waitPoll(t, s); err != nil {
		t.Fatal(err)
	}
	if !ctx.Link.Linked() || ctx.Link.Way() != link.WaySSO || !school(ctx) {
		t.Fatal("not signed in with a school account")
	}
	if resumeSSO(ctx) != nil {
		t.Fatal("a finished sign-in resumes")
	}
}

func waitPoll(t *testing.T, s *SignIn) error {
	t.Helper()
	select {
	case err := <-s.polling:
		s.polling = nil
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("no answer to the poll")
	}
	return nil
}

// TestSignInSSOKeepsAskingWhenTheServerIsSlow checks that a poll that
// got no answer leaves the code on the screen and tries again later, a
// little later each time.
func TestSignInSSOKeepsAskingWhenTheServerIsSlow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"device_code": "hwsso_abc", "user_code": "ABCD-EFGH",
			"verification_uri": "https://halpwords.test/sso/device", "expires_in": 600, "interval": 5})
	}))
	t.Cleanup(srv.Close)
	openPage = func(string) error { return nil }
	t.Cleanup(func() { openPage = browser.Open })
	ctx := testContext(t)
	ctx.Link = link.Open(link.Options{Store: memFiles{}, Server: srv.URL})
	t.Cleanup(ctx.Link.Close)
	s := NewSignIn(ctx, "").(*SignIn)
	s.startSSO(ctx)
	s.answer(ctx, <-s.pending)
	var gaps []uint64
	for range 4 {
		ctx.Tick = s.nextPoll
		s.polled(ctx, context.DeadlineExceeded)
		if s.step != siSSO || s.sso == nil || ctx.Link.PendingSSO() == nil {
			t.Fatalf("gave up after a slow poll: step %d", s.step)
		}
		gaps = append(gaps, s.nextPoll-ctx.Tick)
	}
	for i := 1; i < len(gaps); i++ {
		if gaps[i] <= gaps[i-1] {
			t.Fatalf("no backoff: %v", gaps)
		}
	}
	// Answering again starts over.
	s.polled(ctx, link.ErrSSOPending)
	if s.failed != 0 {
		t.Fatal("the backoff stayed after an answer")
	}
}
