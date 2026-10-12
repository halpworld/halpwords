package scene

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/link"
)

// choicesSite is a server with My choices for a 16-year-old in a family.
type choicesSite struct {
	mu                      sync.Mutex
	stopped, waiting, older bool
	agreed                  bool
	posts                   []string
	noChoices               bool
	// gets counts GET /api/v1/choices; with failAfter set, those after
	// the first failAfter fail (503).
	gets, failAfter int
	// asks are the purposes asked; nil asks rankings only.
	asks []string
}

func (s *choicesSite) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	body, _ := io.ReadAll(r.Body)
	var req map[string]any
	json.Unmarshal(body, &req)
	path := r.URL.Path
	switch {
	case path == "/api/v1/link" || path == "/api/v1/token":
		json.NewEncoder(w).Encode(map[string]any{"device_id": "dev_1", "token_type": "Bearer",
			"access_token": "hwd_1", "expires_in": 86400, "refresh_token": "hwr_1", "refresh_expires_in": 86400})
	case s.noChoices && strings.HasPrefix(path, "/api/v1/choices"):
		w.WriteHeader(http.StatusNotFound)
	case path == "/api/v1/choices":
		s.gets++
		if s.failAfter > 0 && s.gets > s.failAfter {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		// The GET claims the one-time notice, as the server does.
		older := s.older
		s.older = false
		asks := []any{map[string]any{"purpose": "rankings", "agreed": s.agreed, "label": "Put me on the ranking boards",
			"hint": "Your made-up name shows on boards your grown-up turned on."}}
		if s.asks != nil {
			asks = []any{}
			for _, a := range s.asks {
				asks = append(asks, map[string]any{"purpose": a, "agreed": s.agreed, "label": "Ask " + a, "hint": "h"})
			}
		}
		p := map[string]any{"can_stop": !s.stopped, "stopped": s.stopped, "waiting": s.waiting}
		if s.stopped && !s.waiting {
			p["restart"] = map[string]any{"ref": "teen-progress/1/en/r", "title": "Keeping your progress again",
				"body": []string{"You stopped it."}, "label": "I want Halpwords to keep my progress again", "hint": "h"}
		}
		json.NewEncoder(w).Encode(map[string]any{"family": true,
			"child_notice": map[string]any{"ref": "child-link/1/en/c", "title": "Who can see your progress",
				"body": []string{"This game is now linked to Halpwords.", "Your parent can see it."}},
			"older_notice": older,
			"visibility": map[string]any{"ref": "teen-visibility/1/en/v", "title": "Who can see you", "body": []string{"b"},
				"asks": asks},
			"progress": p})
	case strings.HasPrefix(path, "/api/v1/choices/"):
		s.posts = append(s.posts, strings.TrimPrefix(path, "/api/v1/choices/"))
		switch path {
		case "/api/v1/choices/older":
			// A no-op: the GET claimed it.
		case "/api/v1/choices/agreement":
			s.agreed = req["agree"] == true
		case "/api/v1/choices/progress/stop":
			s.stopped = true
		case "/api/v1/choices/progress/start":
			s.waiting = true
		case "/api/v1/choices/progress/cancel":
			s.waiting = false
		}
		w.WriteHeader(http.StatusNoContent)
	case s.stopped:
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "not_linkable", "reason": "progress_stopped"}})
	default:
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

func (s *choicesSite) posted() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.posts...)
}

func choicesContext(t *testing.T, site *choicesSite) *game.Context {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(site.serve))
	t.Cleanup(srv.Close)
	ctx := testContext(t)
	withFont(t, ctx)
	ctx.Link = link.Open(link.Options{Store: memFiles{}, Server: srv.URL})
	if err := ctx.Link.LinkNow(context.Background(), "ABCD-EFGH"); err != nil {
		t.Fatal(err)
	}
	return ctx
}

// chSettle runs the screen until its answer is in.
func chSettle(t *testing.T, ctx *game.Context, c *Choices) {
	t.Helper()
	holdKey(t, ebiten.KeyF24) // nothing pressed
	for deadline := time.Now().Add(5 * time.Second); c.step == chLoading || c.step == chBusy; {
		if time.Now().After(deadline) {
			t.Fatal("no answer")
		}
		c.Update(ctx)
		time.Sleep(5 * time.Millisecond)
	}
}

// chPress presses key once on the screen.
func chPress(t *testing.T, ctx *game.Context, c *Choices, key ebiten.Key) {
	t.Helper()
	holdKey(t, key)
	c.Update(ctx)
	holdKey(t, ebiten.KeyF24)
}

func drawChoices(t *testing.T, ctx *game.Context, c *Choices) {
	t.Helper()
	dst := ebiten.NewImage(game.ScreenW, game.ScreenH)
	c.Draw(dst, ctx)
}

// TestAccountOpensChoicesAfterLinking: once linked, the Account screen
// opens My choices for the child notice and the one-time notice at 16
// or 17; both are told once, then the list shows only what applies.
func TestAccountOpensChoicesAfterLinking(t *testing.T) {
	site := &choicesSite{older: true}
	ctx := choicesContext(t, site)
	a := NewAccount(ctx).(*Account)
	var c *Choices
	for deadline := time.Now().Add(5 * time.Second); c == nil; {
		if time.Now().After(deadline) {
			t.Fatal("My choices not opened")
		}
		c = a.checkChoices(ctx, ctx.Link.Status())
		time.Sleep(5 * time.Millisecond)
	}
	if c.step != chOnPage || c.page.kind != chOlder || !strings.Contains(c.page.body[0], "stop Halpwords keeping your progress") ||
		!strings.Contains(c.page.body[0], "outside your family") {
		t.Fatalf("first: %+v", c.page)
	}
	drawChoices(t, ctx, c)
	chPress(t, ctx, c, ebiten.KeyEnter)
	if c.step != chOnPage || c.page.kind != chChild || c.page.title != "Who can see your progress" {
		t.Fatalf("second: %+v", c.page)
	}
	drawChoices(t, ctx, c)
	chPress(t, ctx, c, ebiten.KeyEnter)
	if c.step != chList {
		t.Fatalf("step %d", c.step)
	}
	drawChoices(t, ctx, c)
	for deadline := time.Now().Add(5 * time.Second); ctx.Link.ChildNoticeDue() || len(site.posted()) < 2; {
		if time.Now().After(deadline) {
			t.Fatalf("not recorded: %v", site.posted())
		}
		time.Sleep(5 * time.Millisecond)
	}
	kinds := []int{}
	for _, it := range c.items() {
		kinds = append(kinds, it.kind)
	}
	if want := []int{chItemNotice, chItemAsk, chItemStop, chItemBack}; !equalInts(kinds, want) {
		t.Errorf("items %v, want %v", kinds, want)
	}

	// Back on the Account screen, nothing is due: it stays.
	a = &Account{checked: false}
	for range 50 {
		if a.checkChoices(ctx, ctx.Link.Status()) != nil {
			t.Fatal("opened again")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestChoicesStopAndRestart: agreeing shows the notice first; stopping
// asks to confirm, keeps the game linked and shows the calm line; asking
// to keep it again waits for a grown-up and can be taken back.
func TestChoicesStopAndRestart(t *testing.T) {
	site := &choicesSite{}
	ctx := choicesContext(t, site)
	ctx.Link.SawNotice(context.Background(), "child-link/1/en/c")
	c := NewChoices(ctx).(*Choices)
	chSettle(t, ctx, c)
	if c.step != chList {
		t.Fatalf("step %d", c.step)
	}

	// Agree to the rankings: the notice, then Y.
	c.sel = 1
	chPress(t, ctx, c, ebiten.KeyEnter)
	if c.step != chOnPage || c.page.kind != chAgree || c.page.purpose != "rankings" {
		t.Fatalf("agree page: %+v", c.page)
	}
	drawChoices(t, ctx, c)
	chPress(t, ctx, c, ebiten.KeyY)
	chSettle(t, ctx, c)
	if !c.ch.Visibility.Asks[0].Agreed {
		t.Error("not agreed")
	}
	// Saying no needs no notice, only a short confirm that says what it
	// does: N keeps yes, Y says no.
	c.sel = 1
	chPress(t, ctx, c, ebiten.KeyEnter)
	if c.step != chWithdraw {
		t.Fatalf("no confirm for saying no: %d", c.step)
	}
	drawChoices(t, ctx, c)
	chPress(t, ctx, c, ebiten.KeyN)
	if c.step != chList || !c.ch.Visibility.Asks[0].Agreed {
		t.Fatalf("N didn't keep yes: %d", c.step)
	}
	chPress(t, ctx, c, ebiten.KeyEnter)
	chPress(t, ctx, c, ebiten.KeyY)
	chSettle(t, ctx, c)
	if c.ch.Visibility.Asks[0].Agreed {
		t.Error("still agreed")
	}

	// Stop: N cancels, Y stops.
	c.sel = 2
	chPress(t, ctx, c, ebiten.KeyEnter)
	if c.step != chStop {
		t.Fatalf("no confirm step: %d", c.step)
	}
	drawChoices(t, ctx, c)
	chPress(t, ctx, c, ebiten.KeyN)
	if c.step != chList || len(site.posted()) != 3 {
		t.Fatalf("N didn't cancel: %d %v", c.step, site.posted())
	}
	chPress(t, ctx, c, ebiten.KeyEnter)
	chPress(t, ctx, c, ebiten.KeyY)
	chSettle(t, ctx, c)
	st := ctx.Link.Status()
	if !st.Linked || !st.Stopped || !c.ch.Progress.Stopped {
		t.Fatalf("after stopping: %+v", st)
	}
	if s, _ := syncProblem(st); !strings.HasPrefix(s, "Your progress isn't being kept") {
		t.Errorf("account line: %q", s)
	}
	drawChoices(t, ctx, c)

	// Keep it again: the notice, Y, then waiting; cancel.
	items := c.items()
	if items[len(items)-2].kind != chItemRestart {
		t.Fatalf("items %+v", items)
	}
	c.sel = len(items) - 2
	chPress(t, ctx, c, ebiten.KeyEnter)
	if c.page.kind != chRestart {
		t.Fatalf("restart page: %+v", c.page)
	}
	chPress(t, ctx, c, ebiten.KeyY)
	chSettle(t, ctx, c)
	if !c.ch.Progress.Waiting || c.items()[c.sel].kind != chItemCancel {
		t.Fatalf("waiting: %+v sel %d", c.ch.Progress, c.sel)
	}
	drawChoices(t, ctx, c)
	chPress(t, ctx, c, ebiten.KeyEnter)
	chSettle(t, ctx, c)
	if c.ch.Progress.Waiting || !ctx.Link.Status().Stopped {
		t.Errorf("after cancelling: %+v", c.ch.Progress)
	}
	want := []string{"notice", "agreement", "agreement", "progress/stop", "progress/start", "progress/cancel"}
	if got := site.posted(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("posted %v, want %v", got, want)
	}
}

// TestChoicesOldServer: a server without My choices hides the Account
// item and says so calmly if My choices is opened anyway.
func TestChoicesOldServer(t *testing.T) {
	site := &choicesSite{noChoices: true}
	ctx := choicesContext(t, site)
	a := NewAccount(ctx).(*Account)
	for deadline := time.Now().Add(5 * time.Second); !a.noChoices; {
		if time.Now().After(deadline) {
			t.Fatal("never heard")
		}
		if a.checkChoices(ctx, ctx.Link.Status()) != nil {
			t.Fatal("opened")
		}
		time.Sleep(5 * time.Millisecond)
	}
	for _, it := range a.items(ctx) {
		if it == acChoices {
			t.Error("My choices offered")
		}
	}
	c := NewChoices(ctx).(*Choices)
	chSettle(t, ctx, c)
	if c.step != chFailed || !strings.Contains(c.msg, "Ask your grown-up") {
		t.Errorf("old server: %d %q", c.step, c.msg)
	}
	drawChoices(t, ctx, c)
}
