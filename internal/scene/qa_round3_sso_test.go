package scene

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/browser"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/move"
)

// qaServer is a school sign-in server: start answers at once unless
// holdStart is set; the token poll answers when tokenGate is closed, with
// tokens (or, when limit is set, a 429 with the Retry-After header).
type qaServer struct {
	mu        sync.Mutex
	holdStart chan struct{}
	tokenGate chan struct{}
	entered   chan string
	unlinks   []string
	polls     []map[string]string
	limit     string // Retry-After of a 429 on poll; "" for tokens
	limited   bool
	startHdr  string
}

func newQAServer(t *testing.T) (*qaServer, *httptest.Server) {
	t.Helper()
	f := &qaServer{entered: make(chan string, 16)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		f.mu.Lock()
		holdStart, gate, limited, limit := f.holdStart, f.tokenGate, f.limited, f.limit
		f.mu.Unlock()
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/api/v1/unlink":
			f.mu.Lock()
			f.unlinks = append(f.unlinks, r.Header.Get("Authorization"))
			f.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		case "/api/v1/sso/start":
			f.entered <- r.URL.Path
			if holdStart != nil {
				<-holdStart
			}
			if limited {
				if limit != "" {
					w.Header().Set("Retry-After", limit)
				}
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "rate_limited", "message": "slow"}})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"device_code": "hwsso_abc", "user_code": "ABCD-EFGH",
				"verification_uri": "https://halpwords.test/sso/device", "expires_in": 600, "interval": 5})
		case "/api/v1/sso/token":
			f.mu.Lock()
			f.polls = append(f.polls, body)
			f.mu.Unlock()
			f.entered <- r.URL.Path
			if gate != nil {
				<-gate
			}
			if limited {
				if limit != "" {
					w.Header().Set("Retry-After", limit)
				}
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "rate_limited", "message": "slow"}})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"device_id": "dev_1", "token_type": "Bearer", "access_token": "hwd_late",
				"expires_in": 3600, "refresh_token": "hwr_late", "refresh_expires_in": 86400})
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *qaServer) unlinked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.unlinks...)
}

func qaWait(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func qaScreen(t *testing.T, srv *httptest.Server) (*SignIn, *game.Context, *lockedFiles) {
	t.Helper()
	openPage = func(string) error { return nil }
	t.Cleanup(func() { openPage = browser.Open })
	ctx := testContext(t)
	ctx.Input = &input.State{}
	files := &lockedFiles{m: memFiles{}}
	ctx.Link = link.Open(link.Options{Store: files, Server: srv.URL})
	t.Cleanup(ctx.Link.Close)
	return NewSignIn(ctx, "").(*SignIn), ctx, files
}

// leaves runs one Update and reports whether the screen tried to leave
// (the test context has no scene stack, so Replace panics).
func leaves(s *SignIn, ctx *game.Context) (left bool) {
	defer func() { left = recover() != nil }()
	s.Update(ctx)
	return false
}

// Esc held through many updates while the school code is asked for: the
// first goes back to the choice (stays), a late code is forgotten, and the
// next leaves the screen, without a crash or a stuck "Asking".
func TestQAEscRepeatedWhileStartingSSO(t *testing.T) {
	f, srv := newQAServer(t)
	f.holdStart = make(chan struct{})
	s, ctx, _ := qaScreen(t, srv)
	s.startSSO(ctx)
	<-f.entered
	press(t, ebiten.KeyEscape)
	if leaves(s, ctx) || s.pending != nil || s.step != siChoose {
		t.Fatalf("first Esc: pending %v step %d", s.pending != nil, s.step)
	}
	close(f.holdStart)
	time.Sleep(100 * time.Millisecond)
	noKeys(t)
	s.Update(ctx)
	if s.step != siChoose || s.sso != nil || ctx.Link.PendingSSO() != nil {
		t.Fatalf("late code kept: step %d sso %v", s.step, ctx.Link.PendingSSO())
	}
	press(t, ebiten.KeyEscape)
	if !leaves(s, ctx) {
		t.Error("the second Esc on the choice should leave")
	}
}

// Esc on the very tick the code's answer is read: the code shows (the
// answer was already in), and the held Esc of the next tick cancels it.
func TestQAEscAsCodeArrives(t *testing.T) {
	f, srv := newQAServer(t)
	s, ctx, _ := qaScreen(t, srv)
	s.startSSO(ctx)
	<-f.entered
	qaWait(t, func() bool { return len(s.pending) == 1 })
	press(t, ebiten.KeyEscape)
	s.Update(ctx) // reads the answer
	if s.step != siSSO || s.sso == nil {
		t.Fatalf("step %d", s.step)
	}
	if leaves(s, ctx) || s.step != siChoose || s.sso != nil || ctx.Link.PendingSSO() != nil {
		t.Fatalf("Esc on the code: step %d pending %v", s.step, ctx.Link.PendingSSO())
	}
	// Mashing Esc on the choice leaves; nothing is left pending.
	if !leaves(s, ctx) {
		t.Error("did not leave")
	}
	if ctx.Link.PendingSSO() != nil {
		t.Error("code pending after leaving")
	}
}

// Esc while a poll's answer is on its way: back to the choice at once, the
// tokens that come late are revoked and the game stays unlinked; further
// Esc presses and ticks don't link or panic.
func TestQAEscDuringPollRevokesLateTokens(t *testing.T) {
	f, srv := newQAServer(t)
	f.tokenGate = make(chan struct{})
	s, ctx, _ := qaScreen(t, srv)
	s.startSSO(ctx)
	<-f.entered
	qaWait(t, func() bool { return len(s.pending) == 1 })
	noKeys(t)
	s.Update(ctx)
	if s.step != siSSO {
		t.Fatalf("step %d", s.step)
	}
	s.nextPoll = 0
	ctx.Tick = 1
	s.Update(ctx)
	<-f.entered // the poll is on the server
	if s.polling == nil {
		t.Fatal("no poll in flight")
	}
	press(t, ebiten.KeyEscape)
	if leaves(s, ctx) || s.step != siChoose || s.polling != nil {
		t.Fatalf("Esc: step %d polling %v", s.step, s.polling != nil)
	}
	noKeys(t)
	close(f.tokenGate)
	qaWait(t, func() bool { return len(f.unlinked()) > 0 })
	for i := 0; i < 5; i++ {
		ctx.Tick += 600
		s.Update(ctx)
	}
	if u := f.unlinked(); ctx.Link.Linked() || len(u) != 1 || u[0] != "Bearer hwd_late" || s.step != siChoose {
		t.Fatalf("linked %v unlinks %q step %d", ctx.Link.Linked(), u, s.step)
	}
}

// Esc while "Signing in…" waits, pressed on every tick: the first goes back
// to the code, the second goes up a step to the choice, and the late
// tokens are revoked exactly once.
func TestQAEscRepeatedWhileSigningIn(t *testing.T) {
	f, srv := newSlowServer(t)
	s, ctx := signInScreen(t, f, srv)
	s.step = siPairing
	s.signIn(ctx, link.SignIn{Code: "ABCD-EFGH-JKLM"})
	<-f.entered
	press(t, ebiten.KeyEscape)
	if leaves(s, ctx) || s.step != siPairing || s.linking {
		t.Fatalf("first Esc: step %d linking %v", s.step, s.linking)
	}
	if leaves(s, ctx) || s.step != siChoose {
		t.Fatalf("second Esc: step %d", s.step)
	}
	noKeys(t)
	f.open()
	qaWait(t, func() bool { f.mu.Lock(); defer f.mu.Unlock(); return len(f.unlinks) > 0 })
	time.Sleep(50 * time.Millisecond)
	for i := 0; i < 3; i++ {
		s.Update(ctx)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if ctx.Link.Linked() || len(f.unlinks) != 1 || s.step != siChoose || ctx.Link.Status().Signing {
		t.Fatalf("linked %v unlinks %q step %d", ctx.Link.Linked(), f.unlinks, s.step)
	}
}

// Esc while the class code is looked up, held: goes back to the choice, the
// late answer is not read, and the next Esc leaves.
func TestQAEscRepeatedWhileAskingForClass(t *testing.T) {
	f, srv := newSlowServer(t)
	s, ctx := signInScreen(t, f, srv)
	s.step, s.classCode = siClass, "C1A55C0D"
	s.ask(ctx, "", "")
	<-f.entered
	press(t, ebiten.KeyEscape)
	if leaves(s, ctx) || s.step != siChoose || s.pending != nil {
		t.Fatalf("step %d pending %v", s.step, s.pending != nil)
	}
	noKeys(t)
	f.open()
	time.Sleep(100 * time.Millisecond)
	s.Update(ctx)
	if s.step != siChoose || s.class != nil {
		t.Fatalf("late answer taken: step %d class %v", s.step, s.class)
	}
	press(t, ebiten.KeyEscape)
	if !leaves(s, ctx) {
		t.Error("the next Esc should leave")
	}
}

// Esc in the other steps (typing a code, the names) is unchanged: one step
// back per press, and a signed-in-nothing screen leaves from the choice.
func TestQAEscStillStepsBack(t *testing.T) {
	_, srv := newQAServer(t)
	s, ctx, _ := qaScreen(t, srv)
	s.step = siPairing
	press(t, ebiten.KeyEscape)
	if leaves(s, ctx) || s.step != siChoose {
		t.Fatalf("step %d", s.step)
	}
	if !leaves(s, ctx) {
		t.Error("choice did not leave")
	}
}

// A rate-limited start shows the wait and stays on the choice; the hint of
// each text is plain English with no website/blame in a remote unlink.
func TestQARateLimitedStartMessage(t *testing.T) {
	for _, tc := range []struct{ hdr, want string }{
		// Short or missing waits are retried by the client with real
		// sleeps (about 4 s): those texts are tested in internal/link.
		{"600", "Too many tries: wait a moment and try again."},
	} {
		t.Run(tc.hdr, func(t *testing.T) {
			f, srv := newQAServer(t)
			f.limited, f.limit = true, tc.hdr
			s, ctx, _ := qaScreen(t, srv)
			s.startSSO(ctx)
			qaWait(t, func() bool { return len(s.pending) == 1 })
			noKeys(t)
			s.Update(ctx)
			if s.step != siChoose || s.msg != tc.want || ctx.Link.PendingSSO() != nil {
				t.Fatalf("step %d msg %q", s.step, s.msg)
			}
		})
	}
}

// A rate-limited poll keeps the code and the screen, shows the text, and
// waits at least as long as the server said (up to a minute).
func TestQARateLimitedPollKeepsScreen(t *testing.T) {
	for _, tc := range []struct {
		hdr  string
		min  time.Duration
		text string
	}{
		{"7", 10 * time.Second, "Too many tries: wait 7 seconds and try again."},
		{"", 10 * time.Second, "Too many tries: wait a moment and try again."},
		{"600", time.Minute, "Too many tries: wait a moment and try again."},
		{"junk", 10 * time.Second, "Too many tries: wait a moment and try again."},
	} {
		t.Run(tc.hdr, func(t *testing.T) {
			f, srv := newQAServer(t)
			s, ctx, _ := qaScreen(t, srv)
			s.startSSO(ctx)
			<-f.entered
			qaWait(t, func() bool { return len(s.pending) == 1 })
			noKeys(t)
			s.Update(ctx)
			f.mu.Lock()
			f.limited, f.limit = true, tc.hdr
			f.mu.Unlock()
			s.nextPoll, ctx.Tick = 0, 1
			s.Update(ctx)
			<-f.entered
			qaWait(t, func() bool { return len(s.polling) == 1 })
			s.Update(ctx)
			if s.step != siSSO || s.sso == nil || ctx.Link.PendingSSO() == nil || s.msg != tc.text {
				t.Fatalf("step %d msg %q pending %v", s.step, s.msg, ctx.Link.PendingSSO())
			}
			if wait := time.Duration(s.nextPoll-ctx.Tick) * time.Second / time.Duration(ebiten.TPS()); wait < tc.min-time.Second || wait > time.Minute+7*time.Second {
				t.Errorf("next poll in %v, want >= %v and <= ~1m", wait, tc.min)
			}
		})
	}
}

// The verifier reaches no scene and no export: the screen's copy of the
// code, the Status, and every file that moves, are free of it, and
// link.json (the only file that holds it) never moves.
func TestQAVerifierNotHandedToScenesOrExport(t *testing.T) {
	f, srv := newQAServer(t)
	f.tokenGate = make(chan struct{})
	s, ctx, files := qaScreen(t, srv)
	s.startSSO(ctx)
	<-f.entered
	qaWait(t, func() bool { return len(s.pending) == 1 })
	noKeys(t)
	s.Update(ctx)
	s.nextPoll, ctx.Tick = 0, 1
	s.Update(ctx)
	<-f.entered
	f.mu.Lock()
	v := f.polls[0]["code_verifier"]
	f.mu.Unlock()
	if len(v) != 43 {
		t.Fatalf("poll verifier %q", v)
	}
	handed := fmt.Sprintf("%+v %+v %+v %+v", s.sso, ctx.Link.PendingSSO(), ctx.Link.Status(), link.PeekFolder(files))
	if strings.Contains(handed, v) {
		t.Errorf("a scene has the verifier: %s", handed)
	}
	raw, _ := files.Read("link.json")
	if !strings.Contains(string(raw), v) {
		t.Error("link.json lacks the verifier: a reload can't go on")
	}
	if move.Exportable("link.json") || move.Exportable("profiles/p1/link.json") {
		t.Error("link.json is exportable")
	}
	// What the export would carry, from every file in the store.
	all := map[string][]byte{}
	files.mu.Lock()
	for n, d := range files.m {
		all[n] = d
	}
	files.mu.Unlock()
	payload, err := move.Encode(all)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(payload, v) {
		t.Error("the verifier is in the export payload")
	}
	// Leave the polling to end.
	press(t, ebiten.KeyEscape)
	s.Update(ctx)
	noKeys(t)
	close(f.tokenGate)
	qaWait(t, func() bool { return len(f.unlinked()) > 0 })
}
