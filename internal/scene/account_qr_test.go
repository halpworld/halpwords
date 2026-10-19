package scene

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/qr"
)

// qrSite is a server that links by QR code, for the Account screen.
type qrSite struct {
	mu       sync.Mutex
	old      bool   // no /link/start: an older server
	busy     bool   // /link/start answers 429 rate_limited
	approved bool   // a grown-up approved the code
	over     string // the poll's answer when the link is over (a code)
	paths    []string
}

func (s *qrSite) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paths = append(s.paths, r.Method+" "+r.URL.Path)
	fail := func(status int, code string) {
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code}})
	}
	switch r.URL.Path {
	case "/api/v1/link/start":
		if s.old {
			fail(http.StatusNotFound, "not_found")
			return
		}
		if s.busy {
			// A long Retry-After: not tried again by the request.
			w.Header().Set("Retry-After", "60")
			fail(http.StatusTooManyRequests, "rate_limited")
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"device_code": "hwlr_1", "user_code": "MKT4-HX7R",
			"verification_uri": "http://" + r.Host + "/link", "verification_uri_complete": "http://" + r.Host + "/link?c=MKT4HX7R",
			"expires_in": 900, "interval": 5})
	case "/api/v1/link/token":
		switch {
		case s.over != "":
			fail(http.StatusForbidden, s.over)
		case !s.approved:
			fail(http.StatusBadRequest, "authorization_pending")
		default:
			json.NewEncoder(w).Encode(map[string]any{"device_id": "dev_1", "token_type": "Bearer", "access_token": "hwd_1",
				"expires_in": 3600, "refresh_token": "hwr_1", "refresh_expires_in": 86400})
		}
	case "/api/v1/me":
		json.NewEncoder(w).Encode(map[string]any{"learner": map[string]any{"id": "lrn_1", "display_name": "Aoife"}})
	case "/api/v1/choices":
		json.NewEncoder(w).Encode(map[string]any{"family": true,
			"child_notice": map[string]any{"ref": "child-link/1/en/c", "title": "Who can see your progress",
				"body": []string{"This game is now linked to Halpwords."}}})
	case "/api/v1/unlink":
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

func (s *qrSite) sent() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.paths...)
}

func (s *qrSite) count(path string) int {
	n := 0
	for _, p := range s.sent() {
		if p == path {
			n++
		}
	}
	return n
}

func qrContext(t *testing.T, site *qrSite) *game.Context {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(site.serve))
	t.Cleanup(srv.Close)
	ctx := testContext(t)
	withFont(t, ctx)
	ctx.Input = &input.State{}
	store := memFiles{}
	ctx.Link = link.Open(link.Options{Store: store, Server: srv.URL})
	t.Cleanup(ctx.Link.Close)
	qrStores[ctx] = qrOpened{store, srv.URL}
	return ctx
}

// qrStores are the link files and server of each qrContext, to open the
// link again as a reload does.
var qrStores = map[*game.Context]qrOpened{}

type qrOpened struct {
	store  memFiles
	server string
}

func qrStore(ctx *game.Context) memFiles { return qrStores[ctx].store }
func qrServer(ctx *game.Context) string  { return qrStores[ctx].server }

// acPress presses key once on the Account screen.
func acPress(t *testing.T, ctx *game.Context, a *Account, key ebiten.Key) {
	t.Helper()
	holdKey(t, key)
	a.Update(ctx)
	ctx.Tick++
	holdKey(t, ebiten.KeyF24)
}

// acUntil runs the Account screen until done, with nothing pressed.
func acUntil(t *testing.T, ctx *game.Context, a *Account, what string, done func() bool) {
	t.Helper()
	holdKey(t, ebiten.KeyF24)
	for deadline := time.Now().Add(5 * time.Second); !done(); {
		if time.Now().After(deadline) {
			t.Fatalf("waiting for %s", what)
		}
		a.Update(ctx)
		ctx.Tick++
		time.Sleep(2 * time.Millisecond)
	}
}

func drawAccount(ctx *game.Context, a *Account) {
	a.Draw(ebiten.NewImage(game.ScreenW, game.ScreenH), ctx)
}

// openLink opens Account → Link to a grown-up's account.
func openLink(t *testing.T, ctx *game.Context) *Account {
	t.Helper()
	a := NewAccount(ctx).(*Account)
	acPress(t, ctx, a, ebiten.KeyEnter)
	if a.mode != acQR {
		t.Fatalf("mode %d, want the QR screen", a.mode)
	}
	return a
}

// TestLinkQROldServer: an older server has no /link/start, so the Link
// screen goes straight to typing a code, with no error, and doesn't ask
// again this session.
func TestLinkQROldServer(t *testing.T) {
	site := &qrSite{old: true}
	ctx := qrContext(t, site)
	a := openLink(t, ctx)
	acUntil(t, ctx, a, "the typed code", func() bool { return a.mode == acCode })
	if a.msg != "" {
		t.Errorf("message %q", a.msg)
	}
	drawAccount(ctx, a)
	acPress(t, ctx, a, ebiten.KeyEscape)
	acPress(t, ctx, a, ebiten.KeyEnter)
	acUntil(t, ctx, a, "the typed code", func() bool { return a.mode == acCode })
	if n := site.count("POST /api/v1/link/start"); n != 1 {
		t.Errorf("%d starts, want 1", n)
	}
}

// TestLinkQRNothingUntilOpened: the game asks for a code only when the
// Link screen opens, polls only while it is open, and forgets the code
// when the child leaves.
func TestLinkQRNothingUntilOpened(t *testing.T) {
	site := &qrSite{}
	ctx := qrContext(t, site)
	a := NewAccount(ctx).(*Account)
	for range 30 {
		a.Update(ctx)
		ctx.Tick++
	}
	drawAccount(ctx, a)
	if s := site.sent(); len(s) != 0 {
		t.Fatalf("sent %v before the Link screen opened", s)
	}
	acPress(t, ctx, a, ebiten.KeyEnter)
	acUntil(t, ctx, a, "a poll", func() bool { return site.count("POST /api/v1/link/token") > 0 && a.pair.polling == nil })
	if a.pair.qr == nil || a.pair.code.UserCode != "MKT4-HX7R" {
		t.Fatalf("no code shown: %+v", a.pair)
	}
	drawAccount(ctx, a)
	acPress(t, ctx, a, ebiten.KeyEscape)
	if a.mode != acBrowse || ctx.Link.PendingPair() != nil {
		t.Fatal("the code is kept after leaving")
	}
	n := len(site.sent())
	for range 400 {
		a.Update(ctx)
		ctx.Tick++
	}
	if len(site.sent()) != n {
		t.Errorf("asked after leaving: %v", site.sent()[n:])
	}

	// A web game that reloads keeps the code in link.json, but asks
	// nothing until the Link screen opens again, and then goes on with
	// the same code.
	acPress(t, ctx, a, ebiten.KeyEnter)
	acUntil(t, ctx, a, "a code", func() bool { return a.pair.code != nil })
	ctx2 := testContext(t)
	withFont(t, ctx2)
	ctx2.Input = &input.State{}
	ctx2.Link = link.Open(link.Options{Store: qrStore(ctx), Server: qrServer(ctx)})
	t.Cleanup(ctx2.Link.Close)
	b := NewAccount(ctx2).(*Account)
	n = len(site.sent())
	for range 400 {
		b.Update(ctx2)
		ctx2.Tick++
	}
	if len(site.sent()) != n {
		t.Errorf("asked before the Link screen opened: %v", site.sent()[n:])
	}
	acPress(t, ctx2, b, ebiten.KeyEnter)
	acUntil(t, ctx2, b, "a poll", func() bool { return len(site.sent()) > n && b.pair.polling == nil })
	if got := site.sent()[n:]; got[0] != "POST /api/v1/link/token" || b.pair.code.UserCode != "MKT4-HX7R" {
		t.Errorf("after a reload: %v", got)
	}
}

// TestLinkQRConfirm: once a grown-up approves, the game asks "Is this
// you?" and shows the child-link notice before anything syncs; the
// first sync is after the notice.
func TestLinkQRConfirm(t *testing.T) {
	site := &qrSite{approved: true}
	ctx := qrContext(t, site)
	a := openLink(t, ctx)
	acUntil(t, ctx, a, "Is this you?", func() bool { return a.mode == acConfirm && a.pair.named })
	if a.pair.name != "Aoife" || !ctx.Link.Status().Unconfirmed {
		t.Fatalf("name %q, %+v", a.pair.name, ctx.Link.Status())
	}
	drawAccount(ctx, a)
	// Esc is "later": back to Account, the link still held, and Finish
	// linking asks again.
	acPress(t, ctx, a, ebiten.KeyEscape)
	if a.mode != acBrowse || !ctx.Link.Status().Unconfirmed {
		t.Fatalf("mode %d, %+v", a.mode, ctx.Link.Status())
	}
	if it := a.items(ctx); it[0] != acFinish {
		t.Fatalf("items %v", it)
	}
	for range 30 {
		a.Update(ctx)
		ctx.Tick++
	}
	drawAccount(ctx, a)
	if a.mode != acBrowse {
		t.Fatal("asked again by itself after Esc")
	}
	acPress(t, ctx, a, ebiten.KeyEnter)
	acUntil(t, ctx, a, "Is this you? from Finish linking", func() bool { return a.mode == acConfirm && a.pair.named })
	// Leaving the screen and coming back asks again.
	a = NewAccount(ctx).(*Account)
	acUntil(t, ctx, a, "Is this you? again", func() bool { return a.mode == acConfirm && a.pair.named })
	// The title screen says so, with Account chosen.
	if ti := NewTitle(ctx).(*Title); ti.items[ti.sel] != titleAccount {
		t.Errorf("title chose %d", ti.items[ti.sel])
	}

	next := ctx.TestScenes(a)
	acPress(t, ctx, a, ebiten.KeyY)
	var c *Choices
	acUntil(t, ctx, a, "the notice", func() bool {
		if s, ok := next().(*Choices); ok {
			c = s
		}
		return c != nil
	})
	if c.step != chOnPage || c.page.kind != chChild {
		t.Fatalf("not the child notice: %+v", c.page)
	}
	drawChoices(t, ctx, c)
	// A sync reads the assigned lists; nothing has.
	if n := site.count("GET /api/v1/lists"); n != 0 || !ctx.Link.Status().Unconfirmed {
		t.Fatalf("synced (%d) or confirmed before the notice", n)
	}
	for _, p := range site.sent() {
		if strings.HasPrefix(p, "POST /api/v1/") && p != "POST /api/v1/link/start" && p != "POST /api/v1/link/token" {
			t.Errorf("sent %s before the notice", p)
		}
	}
	chPress(t, ctx, c, ebiten.KeyEnter)
	if ctx.Link.Status().Unconfirmed {
		t.Fatal("still unconfirmed after the notice")
	}
	for deadline := time.Now().Add(5 * time.Second); site.count("GET /api/v1/lists") == 0; {
		if time.Now().After(deadline) {
			t.Fatalf("no sync after the notice: %v", site.sent())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestLinkQRNotMe: "No" unlinks, keeps everything here and sends
// nothing of the game's.
func TestLinkQRNotMe(t *testing.T) {
	site := &qrSite{approved: true}
	ctx := qrContext(t, site)
	a := openLink(t, ctx)
	acUntil(t, ctx, a, "Is this you?", func() bool { return a.mode == acConfirm && a.pair.named })
	acPress(t, ctx, a, ebiten.KeyN)
	if ctx.Link.Linked() || a.mode != acBrowse || !strings.Contains(a.msg, "Nothing was sent") {
		t.Fatalf("linked %v, mode %d, %q", ctx.Link.Linked(), a.mode, a.msg)
	}
	drawAccount(ctx, a)
	ctx.Link.Close()
	for _, p := range site.sent() {
		switch p {
		case "POST /api/v1/link/start", "POST /api/v1/link/token", "GET /api/v1/me", "POST /api/v1/unlink":
		default:
			t.Errorf("sent %s", p)
		}
	}
}

// TestLinkQRNoticeEsc: Esc on the child-link notice isn't "I've read
// it": the link stays held and nothing syncs.
func TestLinkQRNoticeEsc(t *testing.T) {
	site := &qrSite{approved: true}
	ctx := qrContext(t, site)
	a := openLink(t, ctx)
	acUntil(t, ctx, a, "Is this you?", func() bool { return a.mode == acConfirm && a.pair.named })
	next := ctx.TestScenes(a)
	acPress(t, ctx, a, ebiten.KeyY)
	var c *Choices
	acUntil(t, ctx, a, "the notice", func() bool {
		if s, ok := next().(*Choices); ok {
			c = s
		}
		return c != nil
	})
	chPress(t, ctx, c, ebiten.KeyEscape)
	time.Sleep(50 * time.Millisecond)
	if !ctx.Link.Status().Unconfirmed || site.count("GET /api/v1/lists") != 0 {
		t.Fatalf("Esc on the notice confirmed the link: %+v, %v", ctx.Link.Status(), site.sent())
	}
}

// TestLinkQROver: a code the server says can't be used (the grown-up
// who approved it was removed) says so, with no new code, since one
// wouldn't help; a refused code gets a new one with Enter.
func TestLinkQROver(t *testing.T) {
	site := &qrSite{over: "not_linkable"}
	ctx := qrContext(t, site)
	a := openLink(t, ctx)
	acUntil(t, ctx, a, "the message", func() bool { return a.pair.over != "" })
	if !strings.Contains(a.pair.over, "can't be linked") || a.pair.code != nil || ctx.Link.PendingPair() != nil {
		t.Fatalf("over %q, %+v", a.pair.over, a.pair.code)
	}
	if a.pair.end != pairBack {
		t.Errorf("not_linkable offers %d", a.pair.end)
	}
	drawAccount(ctx, a)
	acPress(t, ctx, a, ebiten.KeyEnter)
	if a.pair.starting != nil || site.count("POST /api/v1/link/start") != 1 {
		t.Fatal("Enter started a new code after not_linkable")
	}

	// Refused: Enter gets a new code.
	site.mu.Lock()
	site.over = "access_denied"
	site.mu.Unlock()
	acPress(t, ctx, a, ebiten.KeyEscape)
	acPress(t, ctx, a, ebiten.KeyEnter)
	acUntil(t, ctx, a, "refused", func() bool { return a.pair.over != "" })
	if !strings.Contains(a.pair.over, "isn't the right game") || a.pair.end != pairNew {
		t.Fatalf("refused: %q, %d", a.pair.over, a.pair.end)
	}
	drawAccount(ctx, a)
	site.mu.Lock()
	site.over = ""
	site.mu.Unlock()
	acPress(t, ctx, a, ebiten.KeyEnter)
	acUntil(t, ctx, a, "a new code", func() bool { return a.pair.code != nil })
	if n := site.count("POST /api/v1/link/start"); n != 3 {
		t.Errorf("%d starts, want 3", n)
	}
	// T types a code instead, and forgets the QR code.
	acPress(t, ctx, a, ebiten.KeyT)
	if a.mode != acCode || ctx.Link.PendingPair() != nil {
		t.Errorf("mode %d", a.mode)
	}
}

// TestQRScale: the code is drawn at a whole number of pixels a module,
// 4 to 6, with its quiet zone, or not at all.
func TestQRScale(t *testing.T) {
	c, err := qr.Encode([]byte("https://test.halpwords.com/link?c=MKT4HX7R"))
	if err != nil {
		t.Fatal(err)
	}
	if s := qrScale(c.Size(), 212); s != 5 {
		t.Errorf("scale %d, want 5 for version %d", s, c.Version())
	}
	if qrScale(c.Size(), 100) != 0 || qrScale(21, 1000) != 6 {
		t.Error("scale limits")
	}
	if countdown(14*time.Minute+59*time.Second+500*time.Millisecond) != "14:59" || countdown(-time.Second) != "0:00" {
		t.Error("countdown")
	}
}

// TestLinkQRStartFails: what a start the server couldn't answer says,
// and that Enter or R tries again; T always types a code.
func TestLinkQRStartFails(t *testing.T) {
	ctx := qrContext(t, &qrSite{})
	for _, c := range []struct {
		err  error
		want string
	}{
		{&link.Error{Status: 400, Code: "invalid_request"}, ""},
		{&link.Error{Status: 503}, "Can't reach the server."},
		{errors.New("dial tcp: refused"), "Can't reach the server."},
	} {
		a := NewAccount(ctx).(*Account)
		a.mode, a.pair = acQR, &acPair{}
		a.started(ctx, pairStarted{err: c.err})
		if a.pair.end != pairRetry || !strings.HasPrefix(a.pair.over, c.want) {
			t.Errorf("%v: %q, %d", c.err, a.pair.over, a.pair.end)
		}
		if c.want == "" && strings.Contains(a.pair.over, "reach") {
			t.Errorf("%v: %q", c.err, a.pair.over)
		}
		drawAccount(ctx, a)
	}
	// A resumed code waits before its first poll.
	p := &acPair{}
	p.show(&link.PairCode{UserCode: "MKT4-HX7R", Every: 5 * time.Second, Expires: time.Now().Add(time.Minute)}, 100)
	if p.nextPoll <= 100 {
		t.Errorf("first poll at tick %d", p.nextPoll)
	}
}

// TestLinkQRBusy: a start the server is too busy for (429) goes
// straight to typing a code, saying why, without asking again by itself;
// the next visit to the screen tries a QR code again.
func TestLinkQRBusy(t *testing.T) {
	site := &qrSite{busy: true}
	ctx := qrContext(t, site)
	a := openLink(t, ctx)
	acUntil(t, ctx, a, "the typed code", func() bool { return a.mode == acCode })
	if !strings.Contains(a.msg, "busy") || !strings.Contains(a.msg, "Show the code") {
		t.Errorf("message %q", a.msg)
	}
	drawAccount(ctx, a)
	for range 400 {
		a.Update(ctx)
		ctx.Tick++
	}
	if n := site.count("POST /api/v1/link/start"); n != 1 {
		t.Fatalf("%d starts, want 1", n)
	}
	site.mu.Lock()
	site.busy = false
	site.mu.Unlock()
	acPress(t, ctx, a, ebiten.KeyEscape)
	acPress(t, ctx, a, ebiten.KeyEnter)
	acUntil(t, ctx, a, "a QR code", func() bool { return a.mode == acQR && a.pair.qr != nil })
	if n := site.count("POST /api/v1/link/start"); n != 2 {
		t.Errorf("%d starts, want 2", n)
	}
}
