package scene

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/qr"
)

// Linking by QR code (halpwords-server#153, #106): Account → Link shows
// a QR code a grown-up scans with a phone, and the code to type on the
// website. The game asks the server for a code only while this screen
// is open, polls only while it is open, and forgets the code when the
// child leaves it. Once a grown-up has approved it, the game asks "Is
// this you?" and shows the child-link notice before anything syncs.

// acPair is the Account screen's link by QR code, from the code to "Is
// this you?".
type acPair struct {
	// stop cancels the start on its way when the child leaves.
	stop     context.CancelFunc
	starting chan pairStarted
	code     *link.PairCode
	qr       *qr.Code // nil when there is nothing to scan
	polling  chan error
	nextPoll uint64
	failed   int // polls in a row the server couldn't answer
	renewed  int // codes started by themselves after one ran out
	// over says why the code ended (Enter gets a new one); offline that
	// the start couldn't reach the server (Enter types a code instead).
	over    string
	offline bool
	note    string // a poll the server couldn't answer: it goes on

	// "Is this you?"
	asking   chan nameResult
	name     string
	named    bool
	nameErr  bool
	checking chan chResult // My choices, after yes
}

type pairStarted struct {
	code *link.PairCode
	err  error
}

type nameResult struct {
	name string
	err  error
}

// pairRenewals is how many codes the screen starts by itself after one
// runs out (3 x 15 minutes), before it waits for Enter.
const pairRenewals = 3

// openPair opens the QR screen: it goes on with a code still on its way
// (a web game that reloaded), or asks for a new one.
func (a *Account) openPair(ctx *game.Context) {
	a.mode, a.pair = acQR, &acPair{}
	a.say("", pal.Steel)
	if p := ctx.Link.PendingPair(); p != nil {
		a.pair.show(p, ctx.Tick)
		a.pair.nextPoll = ctx.Tick // ask at once
		return
	}
	a.pair.start(ctx)
}

// start asks the server for a code, in the background.
func (p *acPair) start(ctx *game.Context) {
	p.code, p.qr, p.over, p.offline, p.note, p.failed = nil, nil, "", false, "", 0
	bg, cancel := context.WithTimeout(context.Background(), askTimeout)
	out := make(chan pairStarted, 1)
	p.stop, p.starting = cancel, out
	go func(lc *link.Client) {
		defer cancel()
		code, err := lc.StartPair(bg)
		out <- pairStarted{code: code, err: err}
	}(ctx.Link)
}

// show takes a code to show.
func (p *acPair) show(code *link.PairCode, tick uint64) {
	p.code, p.qr = code, nil
	if code.ScanURL != "" {
		p.qr, _ = qr.Encode([]byte(code.ScanURL))
	}
	p.nextPoll = tick + ticks(jitter(code.Every))
}

// leavePair gives up the code: the start on its way keeps nothing, a
// poll on its way drops the tokens it gets (and tells the server), and
// nothing more is asked.
func (a *Account) leavePair(ctx *game.Context) {
	if a.pair != nil && a.pair.stop != nil {
		a.pair.stop()
	}
	ctx.Link.CancelPair()
	a.pair = nil
}

func (a *Account) updatePair(ctx *game.Context) {
	p := a.pair
	if p.starting != nil {
		select {
		case r := <-p.starting:
			p.starting = nil
			a.started(ctx, r)
		default:
		}
	}
	if p.polling != nil {
		select {
		case err := <-p.polling:
			p.polling = nil
			a.polled(ctx, err)
		default:
		}
	}
	if a.mode != acQR {
		return
	}
	switch {
	case input.Back():
		ctx.Sound.Play(audio.Back)
		a.leavePair(ctx)
		a.mode = acBrowse
		return
	case typed(ctx, 't', ebiten.KeyT) || p.offline && input.Confirm():
		// Type a grown-up's code: today's typed link.
		ctx.Sound.Play(audio.Select)
		a.leavePair(ctx)
		a.code = a.code[:0]
		a.mode = acCode
		return
	case p.starting == nil && (p.over != "" && input.Confirm() || p.offline && typed(ctx, 'r', ebiten.KeyR)):
		ctx.Sound.Play(audio.Select)
		p.renewed = 0
		p.start(ctx)
		return
	}
	if p.code == nil || p.starting != nil || p.polling != nil {
		return
	}
	if !time.Now().Before(p.code.Expires) {
		a.pairExpired(ctx)
		return
	}
	if ctx.Tick >= p.nextPoll {
		out := make(chan error, 1)
		p.polling = out
		go func(lc *link.Client) {
			bg, cancel := context.WithTimeout(context.Background(), askTimeout)
			defer cancel()
			out <- lc.PollPair(bg)
		}(ctx.Link)
	}
}

// started takes the server's answer to the start.
func (a *Account) started(ctx *game.Context, r pairStarted) {
	p := a.pair
	switch {
	case r.err == nil:
		p.show(r.code, ctx.Tick)
	case errors.Is(r.err, link.ErrPairOff):
		// An older server: today's typed code, with nothing to explain.
		a.pair = nil
		a.code = a.code[:0]
		a.mode = acCode
	case errors.Is(r.err, link.ErrLinked):
		a.pair = nil
		a.mode = acBrowse
	default:
		ctx.Sound.Play(audio.Wrong)
		p.offline = true
		var e *link.Error
		if errors.As(r.err, &e) && e.RetryAfter > 0 {
			p.over = upperFirst(link.Explain(r.err)) + "."
		} else {
			p.over = "Can't reach the server."
		}
	}
}

// polled takes the answer to a poll.
func (a *Account) polled(ctx *game.Context, err error) {
	p := a.pair
	switch {
	case err == nil || errors.Is(err, link.ErrLinked) && ctx.Link.Status().Unconfirmed:
		ctx.Sound.Play(audio.Perfect)
		a.openConfirm(ctx)
	case errors.Is(err, link.ErrLinked):
		a.pair = nil
		a.mode = acBrowse
	case errors.Is(err, link.ErrPairPending):
		p.failed, p.note = 0, ""
		// slow_down makes the game wait longer: take the new interval.
		if pp := ctx.Link.PendingPair(); pp != nil {
			p.code.Every = pp.Every
		}
		p.nextPoll = ctx.Tick + ticks(jitter(p.code.Every))
	case errors.Is(err, link.ErrPairExpired):
		a.pairExpired(ctx)
	case ctx.Link.PendingPair() != nil:
		// The server couldn't be reached: the code still works until it
		// runs out, so go on asking, less and less often.
		p.note = "Can't reach the server. The game keeps trying."
		p.failed++
		wait := pollBackoff(p.code.Every, p.failed)
		var e *link.Error
		if errors.As(err, &e) && e.RetryAfter > 0 {
			wait = max(wait, min(e.RetryAfter, time.Minute))
		}
		p.nextPoll = ctx.Tick + ticks(jitter(wait))
	default:
		// Refused, not linkable, suspended: the code is over.
		ctx.Sound.Play(audio.Wrong)
		p.code, p.qr, p.note = nil, nil, ""
		p.over = upperFirst(link.Explain(err)) + "."
	}
}

// pairExpired starts a new code by itself, a few times, then waits for
// Enter.
func (a *Account) pairExpired(ctx *game.Context) {
	p := a.pair
	ctx.Link.CancelPair()
	if p.renewed < pairRenewals {
		p.renewed++
		p.start(ctx)
		return
	}
	p.code, p.qr, p.note = nil, nil, ""
	p.over = "That code has run out."
}

// openConfirm asks the child whether the learner a grown-up linked the
// game to is them. Nothing syncs until they say yes.
func (a *Account) openConfirm(ctx *game.Context) {
	if a.pair == nil {
		a.pair = &acPair{}
	}
	a.mode = acConfirm
	p := a.pair
	p.code, p.qr, p.over, p.note = nil, nil, "", ""
	p.askName(ctx)
}

func (p *acPair) askName(ctx *game.Context) {
	p.nameErr = false
	out := make(chan nameResult, 1)
	p.asking = out
	go func(lc *link.Client) {
		bg, cancel := context.WithTimeout(context.Background(), askTimeout)
		defer cancel()
		name, err := lc.PairedLearner(bg)
		out <- nameResult{name: name, err: err}
	}(ctx.Link)
}

func (a *Account) updateConfirm(ctx *game.Context) {
	p := a.pair
	if p.asking != nil {
		select {
		case r := <-p.asking:
			p.asking = nil
			p.name, p.named, p.nameErr = r.name, r.err == nil, r.err != nil
			if errors.Is(r.err, link.ErrNotLinked) {
				// Unlinked meanwhile (the server unlinked it).
				a.pair, a.mode = nil, acBrowse
				return
			}
		default:
		}
	}
	if p.checking != nil {
		select {
		case r := <-p.checking:
			p.checking = nil
			a.confirmed(ctx, r)
		default:
		}
		return
	}
	if p.asking != nil {
		return
	}
	// Esc does nothing: the question needs a yes or a no.
	switch {
	case typedNo(ctx):
		// Not them: unlinked, nothing sent, everything kept here.
		ctx.Sound.Play(audio.Back)
		ctx.Link.Unlink()
		a.pair, a.mode, a.sel = nil, acBrowse, 0
		a.say("Unlinked. Nothing was sent. Your progress is still here.", pal.Lime)
	case p.nameErr && input.Confirm():
		ctx.Sound.Play(audio.Select)
		p.askName(ctx)
	case p.named && typedYes(ctx):
		ctx.Sound.Play(audio.Select)
		p.note = ""
		out := make(chan chResult, 1)
		p.checking = out
		go func(lc *link.Client) {
			bg, cancel := context.WithTimeout(context.Background(), askTimeout)
			defer cancel()
			ch, err := lc.Choices(bg)
			out <- chResult{ch: ch, err: err}
		}(ctx.Link)
	}
}

// confirmed takes My choices after the child said yes: the child-link
// notice, when due, shows before the first sync (it confirms the link,
// noticeSeen); else the link is confirmed now and syncs.
func (a *Account) confirmed(ctx *game.Context, r chResult) {
	p := a.pair
	if r.err != nil && !errors.Is(r.err, link.ErrNoChoices) {
		// The notice can't be fetched: nothing syncs yet. Y tries again.
		ctx.Sound.Play(audio.Wrong)
		p.note = upperFirst(link.Explain(r.err)) + "."
		return
	}
	ctx.Sound.Play(audio.Perfect)
	a.linked = "Linked! Your progress now goes to your grown-up's account."
	a.pair, a.mode, a.sel = nil, acBrowse, 0
	a.checked, a.noChoices = true, errors.Is(r.err, link.ErrNoChoices)
	notice := r.err == nil && ctx.Link.ChildNoticeDue() && r.ch.ChildNotice.Ref != ""
	if !notice {
		noticeSeen(ctx)
	}
	if r.err == nil && (notice || r.ch.OlderNotice) {
		msg := a.linked
		a.linked = ""
		ctx.Replace(newChoicesWith(ctx, r.ch, msg))
		return
	}
	a.say(a.linked, pal.Lime)
}

// noticeSeen is the child past the child-link notice (or there was none
// to show): a link by QR code the child said yes to starts syncing.
func noticeSeen(ctx *game.Context) {
	if ctx.Link.ConfirmPair() {
		ctx.Link.SyncNow()
	}
}

// qrScale is how many pixels each module of a code size modules wide
// gets, with the quiet zone, in room pixels: at least 4, or 0 when the
// code doesn't fit.
func qrScale(size, room int) int {
	s := min(room/(size+8), 6)
	if s < 4 {
		return 0
	}
	return s
}

// drawQR draws c dark on light, with a 4-module quiet zone, at x, y with
// scale pixels a module, and returns its width.
func drawQR(dst *ebiten.Image, c *qr.Code, x, y, scale int) int {
	n := c.Size()
	w := (n + 8) * scale
	gfx.FillRect(dst, x, y, w, w, pal.White)
	for my := range n {
		for mx := 0; mx < n; {
			if !c.Dark(mx, my) {
				mx++
				continue
			}
			// A run of dark modules is one rectangle.
			run := 1
			for mx+run < n && c.Dark(mx+run, my) {
				run++
			}
			gfx.FillRect(dst, x+(4+mx)*scale, y+(4+my)*scale, run*scale, scale, pal.Black)
			mx += run
		}
	}
	return w
}

// countdown says how long a code still works: 14:59.
func countdown(left time.Duration) string {
	s := max(int(left.Seconds()), 0)
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// drawPair draws the QR screen, with its key hint.
func (a *Account) drawPair(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	p := a.pair
	const w, h = 600, 300
	dx, dy := dialog(dst, ctx, "LINK THIS GAME", w, h)
	right := game.ScreenW/2 + w/2 - 20
	hint := "T type a grown-up's code   Esc back"
	switch {
	case p.offline:
		f.DrawShadow(dst, p.over, dx, dy, 1, pal.Rose)
		f.DrawShadow(dst, "You can type a code from the website instead.", dx, dy+24, 1, pal.Ice)
		hint = "Enter type a code   R try again   Esc back"
	case p.over != "":
		for i, l := range wrap(f, p.over, right-dx) {
			f.DrawShadow(dst, l, dx, dy+i*18, 1, pal.Rose)
		}
		f.DrawShadow(dst, "Press Enter for a new code.", dx, dy+60, 1, pal.Ice)
		hint = "Enter new code   T type a grown-up's code   Esc back"
	case p.code == nil:
		f.DrawShadow(dst, "Getting a code…", dx, dy, 1, pal.Ice)
	default:
		tx := dx
		if p.qr != nil {
			if s := qrScale(p.qr.Size(), h-48-40); s > 0 {
				tx = dx + drawQR(dst, p.qr, dx, dy, s) + 20
			}
		}
		tw := right - tx
		y := dy
		f.DrawShadow(dst, fit(f, "Ask a grown-up to scan this", tw, 1), tx, y, 1, pal.Yellow)
		f.DrawShadow(dst, fit(f, "with their phone's camera,", tw, 1), tx, y+18, 1, pal.Ice)
		addr := strings.TrimPrefix(strings.TrimPrefix(p.code.URL, "https://"), "http://")
		f.DrawShadow(dst, fit(f, "or go to "+addr, tw, 1), tx, y+36, 1, pal.Ice)
		f.DrawShadow(dst, fit(f, "and type this code:", tw, 1), tx, y+54, 1, pal.Ice)
		f.DrawShadow(dst, p.code.UserCode, tx, y+78, 3, pal.White)
		left := time.Until(p.code.Expires)
		c := pal.Steel
		if left < time.Minute {
			c = pal.Tan
		}
		f.DrawShadow(dst, "The code works for "+countdown(left), tx, y+130, 1, c)
		if p.note != "" {
			for i, l := range wrap(f, p.note, tw) {
				f.DrawShadow(dst, l, tx, y+154+i*18, 1, pal.Tan)
			}
		}
	}
	f.DrawShadow(dst, hint, dx, dy+h-48-24, 1, pal.Ash)
}

// drawConfirm draws "Is this you?".
func (a *Account) drawConfirm(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	p := a.pair
	const w = 480
	dx, dy := dialog(dst, ctx, "IS THIS YOU?", w, 190)
	switch {
	case p.asking != nil:
		f.DrawShadow(dst, "A grown-up linked this game. Checking who to…", dx, dy, 1, pal.Ice)
	case p.nameErr:
		f.DrawShadow(dst, "A grown-up linked this game, but the game", dx, dy, 1, pal.Ice)
		f.DrawShadow(dst, "couldn't check who to. Nothing was sent.", dx, dy+18, 1, pal.Ice)
		f.DrawShadow(dst, "Enter try again   N unlink", dx, dy+100, 1, pal.Ash)
	default:
		name := p.name
		if name == "" {
			name = "a learner with no name"
		}
		f.DrawShadow(dst, fit(f, "Linked to "+name+".", w-40, 1), dx, dy, 1, pal.White)
		f.DrawShadow(dst, "Is this you?", dx, dy+22, 2, pal.Yellow)
		f.DrawShadow(dst, "Nothing is sent until you say yes.", dx, dy+56, 1, pal.Steel)
		if p.checking != nil {
			f.DrawShadow(dst, "Linking…", dx, dy+76, 1, pal.Ice)
		} else if p.note != "" {
			f.DrawShadow(dst, fit(f, p.note, w-40, 1), dx, dy+76, 1, pal.Tan)
		}
		f.DrawShadow(dst, "Y yes, that's me   N no, unlink", dx, dy+100, 1, pal.Ash)
	}
}
