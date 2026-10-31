package scene

import (
	"context"
	"errors"
	"fmt"
	"image/color"
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

// pairEnd is what a code that ended lets the child do.
type pairEnd int

const (
	pairRetry pairEnd = iota + 1 // Enter tries again (the server couldn't answer)
	pairNew                      // Enter gets a new code
	pairBack                     // only back, or type a code
)

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
	failed   int    // polls in a row the server couldn't answer
	renewed  int    // codes started by themselves after one ran out
	fresh    uint64 // "Here's a new code." shows until this tick
	// over says why there is no code, and end what the child can do.
	over string
	end  pairEnd
	note string // a poll the server couldn't answer: it goes on

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

// freshTicks is how long "Here's a new code." shows: 3 seconds.
const freshTicks = 180

// openPair opens the QR screen: it goes on with a code still on its way
// (a web game that reloaded), or asks for a new one.
func (a *Account) openPair(ctx *game.Context) {
	a.mode, a.pair = acQR, &acPair{}
	a.say("", pal.Steel)
	if p := ctx.Link.PendingPair(); p != nil {
		a.pair.show(p, ctx.Tick)
		return
	}
	a.pair.start(ctx)
}

// start asks the server for a code, in the background. Its ctx is only
// cancelled (the child left), never timed: each request has its own
// timeout, and an answer that comes in time is kept.
func (p *acPair) start(ctx *game.Context) {
	p.code, p.qr, p.over, p.end, p.note, p.failed = nil, nil, "", 0, "", 0
	bg, cancel := context.WithCancel(context.Background())
	out := make(chan pairStarted, 1)
	p.stop, p.starting = cancel, out
	go func(lc *link.Client) {
		defer cancel()
		code, err := lc.StartPair(bg)
		out <- pairStarted{code: code, err: err}
	}(ctx.Link)
}

// show takes a code to show; the first poll is after the interval.
func (p *acPair) show(code *link.PairCode, tick uint64) {
	p.code, p.qr = code, nil
	if code.ScanURL != "" {
		p.qr, _ = qr.Encode([]byte(code.ScanURL))
	}
	p.nextPoll = tick + ticks(jitter(code.Every))
}

// ended shows why there is no code.
func (p *acPair) ended(msg string, end pairEnd) {
	p.code, p.qr, p.note, p.fresh = nil, nil, "", 0
	p.over, p.end = msg, end
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
	retry := input.Confirm() || typed(ctx, 'r', ebiten.KeyR)
	switch {
	case input.Back():
		ctx.Sound.Play(audio.Back)
		a.leavePair(ctx)
		a.mode = acBrowse
		return
	case typed(ctx, 't', ebiten.KeyT):
		// Type a grown-up's code: today's typed link.
		ctx.Sound.Play(audio.Select)
		a.leavePair(ctx)
		a.code = a.code[:0]
		a.mode = acCode
		return
	case p.starting == nil && (p.end == pairRetry && retry || p.end == pairNew && input.Confirm()):
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
			// Not timed beyond each request's own timeout: tokens that
			// come back are kept, or revoked if the child left.
			out <- lc.PollPair(context.Background())
		}(ctx.Link)
	}
}

// started takes the server's answer to the start.
func (a *Account) started(ctx *game.Context, r pairStarted) {
	p := a.pair
	var e *link.Error
	switch {
	case r.err == nil:
		p.show(r.code, ctx.Tick)
		if p.renewed > 0 {
			p.fresh = ctx.Tick + freshTicks
		}
	case errors.Is(r.err, link.ErrPairOff):
		// An older server: today's typed code, with nothing to explain.
		a.pair = nil
		a.code = a.code[:0]
		a.mode = acCode
	case errors.Is(r.err, link.ErrLinked):
		a.pair = nil
		a.mode = acBrowse
	case errors.Is(r.err, link.ErrSSOBusy) || errors.As(r.err, &e) && e.Status == 429:
		// Too many codes asked for (here, or server-wide): the typed code
		// works without one. Not remembered: the next visit tries QR again.
		ctx.Sound.Play(audio.Wrong)
		a.pair = nil
		a.code = a.code[:0]
		a.mode = acCode
		a.say("The server is busy, so type a code instead: your grown-up chooses Show the code on the website.", pal.Tan)
	default:
		ctx.Sound.Play(audio.Wrong)
		switch {
		case errors.As(r.err, &e) && e.Status < 500:
			p.ended(upperFirst(link.Explain(r.err))+".", pairRetry)
		default:
			p.ended("Can't reach the server.", pairRetry)
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
		ctx.Sound.Play(audio.Wrong)
		switch {
		case errors.Is(err, link.ErrPairRefused):
			p.ended("Your grown-up said this isn't the right game. Ask them to try again.", pairNew)
		case errors.Is(err, link.ErrSuspended):
			// A new code wouldn't help.
			p.ended("This account is paused. Ask your grown-up.", pairBack)
		case errors.Is(err, link.ErrNotLinkable):
			p.ended(upperFirst(link.Explain(err))+".", pairBack)
		default:
			p.ended(upperFirst(link.Explain(err))+".", pairNew)
		}
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
	p.ended("That code has run out.", pairNew)
}

// openConfirm asks the child whether the learner a grown-up linked the
// game to is them. Nothing syncs until they say yes.
func (a *Account) openConfirm(ctx *game.Context) {
	a.mode, a.pair = acConfirm, &acPair{}
	a.pair.askName(ctx)
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

// askChoices fetches My choices after yes, for the child-link notice.
func (p *acPair) askChoices(ctx *game.Context) {
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
			if a.mode != acConfirm {
				return
			}
		default:
		}
	}
	waiting := p.asking != nil || p.checking != nil
	switch {
	case typedNo(ctx):
		// Not them: unlinked, nothing sent, everything kept here.
		ctx.Sound.Play(audio.Back)
		ctx.Link.Unlink()
		a.pair, a.mode, a.sel = nil, acBrowse, 0
		a.say("Unlinked. Nothing was sent. Your progress is still here.", pal.Lime)
	case input.Back():
		// Later: the link stays held, and Account asks again.
		ctx.Sound.Play(audio.Back)
		a.pair, a.mode, a.sel, a.later = nil, acBrowse, 0, true
	case waiting:
	case p.nameErr && input.Confirm():
		ctx.Sound.Play(audio.Select)
		p.askName(ctx)
	case p.named && (typedYes(ctx) || p.note != "" && input.Confirm()):
		ctx.Sound.Play(audio.Select)
		p.askChoices(ctx)
	}
}

// confirmed takes My choices after the child said yes: the child-link
// notice, when due, shows before the first sync (Enter on it confirms
// the link, noticeSeen); else the link is confirmed now and syncs.
func (a *Account) confirmed(ctx *game.Context, r chResult) {
	p := a.pair
	if r.err != nil && !errors.Is(r.err, link.ErrNoChoices) {
		// The notice can't be fetched: nothing syncs yet. Y tries again.
		ctx.Sound.Play(audio.Wrong)
		p.note = upperFirst(link.Explain(r.err)) + ". Press Y to try again."
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

// noticeSeen is the child past the child-link notice with "I've read
// it" (or there was none to show): a link by QR code the child said yes
// to starts syncing.
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

// wrap2 wraps s to width in at most two lines; a longer one is cut
// short.
func wrap2(f *gfx.Font, s string, width int) []string {
	lines := wrapHard(f, s, width)
	if len(lines) > 2 {
		lines = []string{lines[0], strings.Join(lines[1:], " ")}
	}
	if len(lines) > 0 {
		lines[len(lines)-1] = fit(f, lines[len(lines)-1], width, 1)
	}
	return lines
}

// drawLines draws lines from y, and returns the y after them.
func drawLines(dst *ebiten.Image, f *gfx.Font, lines []string, x, y int, c color.Color) int {
	for _, l := range lines {
		f.DrawShadow(dst, l, x, y, 1, c)
		y += 18
	}
	return y
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
	case p.over != "":
		drawLines(dst, f, wrap2(f, p.over, right-dx), dx, dy, pal.Yellow)
		switch p.end {
		case pairRetry:
			hint = "Enter try again   T type a grown-up's code   Esc back"
		case pairNew:
			hint = "Enter new code   T type a grown-up's code   Esc back"
		}
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
		y := drawLines(dst, f, []string{fit(f, "Ask a grown-up to scan this", tw, 1)}, tx, dy, pal.Yellow)
		addr := strings.TrimPrefix(strings.TrimPrefix(p.code.URL, "https://"), "http://")
		y = drawLines(dst, f, append([]string{fit(f, "with their phone's camera,", tw, 1)},
			wrap2(f, "or go to "+addr, tw)...), tx, y, pal.Ice)
		y = drawLines(dst, f, []string{fit(f, "and type this code:", tw, 1)}, tx, y, pal.Ice)
		f.DrawShadow(dst, p.code.UserCode, tx, y+6, 3, pal.White)
		y += 6 + 3*gfx.LineHeight + 6
		switch {
		case p.note != "":
			y = drawLines(dst, f, wrap2(f, p.note, tw), tx, y, pal.Tan)
		case ctx.Tick < p.fresh:
			y = drawLines(dst, f, []string{fit(f, "Here's a new code.", tw, 1)}, tx, y, pal.Lime)
		default:
			y = drawLines(dst, f, []string{fit(f, "Waiting for a grown-up to scan…", tw, 1)}, tx, y, pal.Ice)
		}
		left := time.Until(p.code.Expires)
		c := pal.Steel
		if left < time.Minute {
			c = pal.Tan
		}
		f.DrawShadow(dst, fit(f, "The code works for "+countdown(left), tw, 1), tx, y, 1, c)
	}
	f.DrawShadow(dst, hint, dx, dy+h-48-24, 1, pal.Steel)
}

// pairedWho is the line "Is this you?" asks about: the learner's name,
// and where they belong when the server said ("family" or "school"), so
// a child with the same name in both picks the right one (halpwords#110).
func pairedWho(name, where string) string {
	switch {
	case name == "":
		return "Linked, but no name was set."
	case where == "":
		return "Linked to " + name + "."
	}
	return "Linked to " + name + " (" + where + ")."
}

// drawConfirm draws "Is this you?".
func (a *Account) drawConfirm(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	p := a.pair
	const w, h = 480, 210
	dx, dy := dialog(dst, ctx, "NEW LINK", w, h)
	tw := w - 40
	hint := "Y yes, that's me   N no, it isn't me   Esc later"
	switch {
	case p.asking != nil:
		drawLines(dst, f, wrap2(f, "A grown-up linked this game. Checking who to…", tw), dx, dy, pal.Ice)
		hint = "N no, unlink   Esc later"
	case p.nameErr:
		drawLines(dst, f, wrap2(f, "A grown-up linked this game, but the game couldn't check who to. Nothing was sent.", tw),
			dx, dy, pal.Yellow)
		hint = "Enter try again   N no, unlink   Esc later"
	default:
		who := pairedWho(p.name, ctx.Link.PairedWhere())
		y := drawLines(dst, f, wrap2(f, who, tw), dx, dy, pal.White)
		f.DrawShadow(dst, "Is this you?", dx, y+4, 2, pal.Yellow)
		y += 4 + 2*gfx.LineHeight + 6
		y = drawLines(dst, f, []string{"Nothing is sent until you say yes."}, dx, y, pal.Steel)
		switch {
		case p.checking != nil:
			drawLines(dst, f, []string{"Linking…"}, dx, y, pal.Ice)
			hint = "N no, it isn't me   Esc later"
		case p.note != "":
			drawLines(dst, f, wrap2(f, p.note, tw), dx, y, pal.Yellow)
		}
	}
	f.DrawShadow(dst, hint, dx, dy+h-48-24, 1, pal.Steel)
}
