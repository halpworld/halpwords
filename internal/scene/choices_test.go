package scene

import (
	"context"
	"encoding/json"
	"fmt"
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
	// young: an 8-12, who can only ask their grown-up to stop keeping
	// their progress (servers from 2026-10-13; oldAsk: a server from
	// before, which doesn't say); asked and kept are where that stands.
	young, oldAsk, asked, kept bool
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
		p := map[string]any{"can_stop": !s.stopped && !s.young, "stopped": s.stopped, "waiting": s.waiting}
		if s.young && !s.oldAsk {
			a := map[string]any{"can_ask": !s.asked && !s.kept, "kept": s.kept}
			if s.asked {
				a["waiting_since"] = "2026-10-13T09:30:00Z"
			}
			p["ask"] = a
		}
		if s.stopped && !s.waiting {
			p["restart"] = map[string]any{"ref": "teen-progress/1/en/r", "title": "Keeping your progress again",
				"body": []string{"You stopped it."}, "label": "I want Halpwords to keep my progress again", "hint": "h"}
		}
		vis := map[string]any{"ref": "teen-visibility/1/en/v", "title": "Who can see you", "body": []string{"b"}, "asks": asks}
		if len(asks) == 0 {
			// Nothing asked: no notice, as the server sends it.
			vis = map[string]any{"asks": asks}
		}
		out := map[string]any{"family": true,
			"child_notice": map[string]any{"ref": "child-link/1/en/c", "title": "Who can see your progress",
				"body": []string{"This game is now linked to Halpwords.", "Your parent can see it."}},
			"older_notice": older,
			"visibility":   vis,
			"progress":     p}
		if s.young {
			// An 8-12 decides nothing about who sees them.
			delete(out, "visibility")
		}
		json.NewEncoder(w).Encode(out)
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
		case "/api/v1/choices/progress/ask":
			s.asked = req["confirm"] == true
		case "/api/v1/choices/progress/ask/cancel":
			s.asked = false
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

// chInfo is the list's window text.
func chInfo(ctx *game.Context, c *Choices) string {
	var b strings.Builder
	for _, l := range c.infoLines(ctx.Font, ctx.Link.Status(), game.ScreenW-96) {
		b.WriteString(l.s + " ")
	}
	return b.String()
}

func chKinds(c *Choices) []int {
	var out []int
	for _, it := range c.items() {
		out = append(out, it.kind)
	}
	return out
}

// TestChoicesAskGrownUp: a child under 16 asks their grown-up to stop
// keeping their progress (halpwords-server PLAN §25, 2026-10-13): Y/N
// first (N asks nothing), then the list says when they asked and offers
// to take it back; when a grown-up keeps it, the child is told only
// that. The game keeps syncing. A server from before shows nothing new.
func TestChoicesAskGrownUp(t *testing.T) {
	site := &choicesSite{young: true}
	ctx := choicesContext(t, site)
	ctx.Link.SawNotice(context.Background(), "child-link/1/en/c")
	c := NewChoices(ctx).(*Choices)
	chSettle(t, ctx, c)
	if got := chKinds(c); fmt.Sprint(got) != fmt.Sprint([]int{chItemNotice, chItemAskStop, chItemBack}) {
		t.Fatalf("items %v", got)
	}
	if info := chInfo(ctx, c); !strings.Contains(info, "you can ask your grown-up") {
		t.Errorf("info: %q", info)
	}
	drawChoices(t, ctx, c)

	c.sel = 1
	chPress(t, ctx, c, ebiten.KeyEnter)
	if c.step != chAskStop {
		t.Fatalf("no Y/N step: %d", c.step)
	}
	drawChoices(t, ctx, c)
	chPress(t, ctx, c, ebiten.KeyN)
	if c.step != chList || len(site.posted()) != 1 {
		t.Fatalf("N asked: %d %v", c.step, site.posted())
	}
	chPress(t, ctx, c, ebiten.KeyEnter)
	chPress(t, ctx, c, ebiten.KeyY)
	chSettle(t, ctx, c)
	if !strings.Contains(c.msg, "We emailed your grown-up") {
		t.Errorf("message: %q", c.msg)
	}
	if info := chInfo(ctx, c); !strings.Contains(info, "You asked your grown-up on 13 October 2026") &&
		!strings.Contains(info, "You asked your grown-up on 14 October 2026") && !strings.Contains(info, "You asked your grown-up on 12 October 2026") {
		t.Errorf("waiting info: %q", info)
	}
	if st := ctx.Link.Status(); st.Stopped || !st.Linked {
		t.Errorf("the ask changed the link: %+v", st)
	}
	if c.items()[c.sel].kind != chItemTakeBack {
		t.Fatalf("items after asking %v sel %d", chKinds(c), c.sel)
	}
	drawChoices(t, ctx, c)
	chPress(t, ctx, c, ebiten.KeyEnter)
	chSettle(t, ctx, c)
	if site.asked || c.ch.Progress.Ask.CanAsk != true {
		t.Errorf("after taking back: %+v", c.ch.Progress.Ask)
	}
	if want := "notice,progress/ask,progress/ask/cancel"; strings.Join(site.posted(), ",") != want {
		t.Errorf("posted %v, want %s", site.posted(), want)
	}

	// Kept: told only that, with nothing to ask.
	site.mu.Lock()
	site.kept = true
	site.mu.Unlock()
	c.load(ctx, "")
	chSettle(t, ctx, c)
	if info := chInfo(ctx, c); !strings.Contains(info, "Your grown-up decided to keep your progress. You can talk to them about it.") {
		t.Errorf("kept info: %q", info)
	}
	if got := chKinds(c); fmt.Sprint(got) != fmt.Sprint([]int{chItemNotice, chItemBack}) {
		t.Errorf("items when kept %v", got)
	}
	drawChoices(t, ctx, c)

	// A server from before 2026-10-13 says nothing about asking.
	old := &choicesSite{young: true, oldAsk: true}
	octx := choicesContext(t, old)
	octx.Link.SawNotice(context.Background(), "child-link/1/en/c")
	oc := NewChoices(octx).(*Choices)
	chSettle(t, octx, oc)
	if got := chKinds(oc); fmt.Sprint(got) != fmt.Sprint([]int{chItemNotice, chItemBack}) {
		t.Errorf("items from an old server %v", got)
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
