package scene

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/pal"
)

// My choices (halpwords-server PLAN §25, 2026-10-12; consent rule R6):
// in a linked game, who can see the learner's progress, and what a
// teenager in a family can decide themself: being in friend groups and
// on ranking boards (13-17), and, at 16 or 17, stopping Halpwords keeping
// their progress; under 16, asking their grown-up to stop it
// (halpwords-server PLAN §25, 2026-10-13). Only what applies is shown. The words of the notices
// come from the server, so they are the ones the website shows.

// chStep is what My choices is doing.
type chStep int

const (
	chLoading  chStep = iota
	chFailed          // couldn't load, or loading again failed: msg (and msg2) say why
	chOnPage          // showing a notice
	chList            // the choices
	chStop            // confirming a stop
	chWithdraw        // confirming saying no to something agreed
	chAskStop         // confirming asking a grown-up to stop keeping progress
	chBusy            // an answer is on its way
)

// chPageKind is the notice a page shows, and what Enter or Y does.
type chPageKind int

const (
	chChild   chPageKind = iota // who can see your progress (told once per link)
	chOlder                     // the one-time notice at 16 or 17
	chAgree                     // the teen-visibility notice, before agreeing
	chRestart                   // the teen-progress notice, before asking to keep it again
	chRead                      // the child notice again, from the list
)

type chPage struct {
	kind    chPageKind
	title   string
	body    []string
	ref     string
	purpose string // chAgree: what is agreed to
	label   string // chAgree, chRestart: what Y does
}

// chItem is an entry in the list of choices; ask is the purpose's index.
type chItem struct {
	kind int
	ask  int
}

const (
	chItemNotice = iota
	chItemAsk
	chItemStop
	chItemRestart
	chItemCancel
	chItemAskStop  // under 16: ask a grown-up to stop keeping progress
	chItemTakeBack // take that ask back
	chItemBack
)

// chResult is the answer to a request made in the background.
type chResult struct {
	ch   *link.Choices
	err  error
	done string // what to say when it worked
	// acted: the request itself worked, and err is from fetching the
	// choices again afterwards.
	acted bool
}

// Choices is the My choices screen.
type Choices struct {
	bg     *ebiten.Image
	step   chStep
	sel    int
	top    int // the first item the list shows
	scroll int
	ch     *link.Choices
	// pages are notices to show before the list: the one at 16 or 17,
	// the child notice.
	pages []chPage
	page  chPage
	out   chan chResult
	// still: Esc was pressed while an answer was on its way.
	still  bool
	msg    string
	msgCol color.RGBA
	// msg2 is a second message, after msg: a problem fetching the
	// choices again after a change that worked.
	msg2 string
}

// NewChoices opens My choices and fetches them.
func NewChoices(ctx *game.Context) game.Scene {
	c := &Choices{bg: backdrop(11, 1.2)}
	c.load(ctx, "")
	return c
}

// newChoicesWith opens My choices with choices already fetched, such as
// the Account screen's, to tell what is due. The one-time notice at 16
// or 17 is shown from these: the server counted it as told when it
// answered. msg, if any, shows on the first page ("Linked!").
func newChoicesWith(ctx *game.Context, ch *link.Choices, msg string) *Choices {
	c := &Choices{bg: backdrop(11, 1.2)}
	c.loaded(ctx, ch)
	if msg != "" {
		c.say(msg, pal.Lime)
	}
	return c
}

func (c *Choices) say(msg string, col color.RGBA) { c.msg, c.msgCol, c.msg2 = msg, col, "" }

// run makes a request in the background, then fetches the choices again.
func (c *Choices) run(ctx *game.Context, done string, do func(context.Context, *link.Client) error) {
	var with func(context.Context, *link.Client) (string, error)
	if do != nil {
		with = func(bg context.Context, lc *link.Client) (string, error) { return done, do(bg, lc) }
	}
	c.runWith(ctx, done, with)
}

// runWith is run whose action says what to tell the learner when it
// worked.
func (c *Choices) runWith(ctx *game.Context, done string, do func(context.Context, *link.Client) (string, error)) {
	lc := ctx.Link
	out := make(chan chResult, 1)
	c.out, c.step, c.still = out, chBusy, false
	go func() {
		bg, cancel := context.WithTimeout(context.Background(), askTimeout)
		defer cancel()
		if do != nil {
			msg, err := do(bg, lc)
			if err != nil {
				out <- chResult{err: err}
				return
			}
			done = msg
		}
		ch, err := lc.Choices(bg)
		out <- chResult{ch: ch, err: err, done: done, acted: do != nil}
	}()
}

func (c *Choices) load(ctx *game.Context, done string) {
	c.run(ctx, done, nil)
	c.step = chLoading
}

// loaded takes fetched choices: the notices due first, then the list.
// The one at 16 or 17 comes first: the server counted it as told when
// it answered, so it is never lost behind the child notice.
func (c *Choices) loaded(ctx *game.Context, ch *link.Choices) {
	c.ch, c.pages = ch, nil
	if ch.OlderNotice {
		c.pages = append(c.pages, chPage{kind: chOlder, title: "You can decide more for yourself now",
			body: []string{olderText(ch)}})
	}
	if ctx.Link.ChildNoticeDue() && ch.ChildNotice.Ref != "" {
		c.pages = append(c.pages, chPage{kind: chChild, title: ch.ChildNotice.Title, body: ch.ChildNotice.Body,
			ref: ch.ChildNotice.Ref})
	}
	c.next()
}

// olderText is the one-time notice at 16 or 17, naming only what the
// learner can choose (as the website does).
func olderText(ch *link.Choices) string {
	stop := ch.Progress != nil && ch.Progress.CanStop
	seen := ch.Visibility != nil && len(ch.Visibility.Asks) > 0
	switch {
	case stop && seen:
		return "Now that you are 16 or 17, you can stop Halpwords keeping your progress yourself, and decide who outside your family can see you."
	case stop:
		return "Now that you are 16 or 17, you can stop Halpwords keeping your progress yourself."
	case seen:
		return "Now that you are 16 or 17, you can decide who outside your family can see you."
	}
	return "Now that you are 16 or 17, you can decide more for yourself. See what you can choose."
}

// next shows the next notice due, or the list.
func (c *Choices) next() {
	c.scroll = 0
	if len(c.pages) > 0 {
		c.page, c.pages = c.pages[0], c.pages[1:]
		c.step = chOnPage
		return
	}
	c.step = chList
}

func (c *Choices) items() []chItem {
	var items []chItem
	items = append(items, chItem{kind: chItemNotice})
	if c.ch != nil && c.ch.Visibility != nil {
		for i := range c.ch.Visibility.Asks {
			items = append(items, chItem{kind: chItemAsk, ask: i})
		}
	}
	if p := c.progress(); p != nil {
		switch {
		case p.CanStop:
			items = append(items, chItem{kind: chItemStop})
		case p.Waiting:
			items = append(items, chItem{kind: chItemCancel})
		case p.Stopped && p.Restart != nil:
			// Only when asking can work (the server says so): otherwise
			// the list says to ask a parent or guardian.
			items = append(items, chItem{kind: chItemRestart})
		case p.Ask != nil && !p.Ask.WaitingSince.IsZero():
			items = append(items, chItem{kind: chItemTakeBack})
		case p.Ask != nil && p.Ask.CanAsk:
			items = append(items, chItem{kind: chItemAskStop})
		}
	}
	return append(items, chItem{kind: chItemBack})
}

func (c *Choices) progress() *link.OwnProgress {
	if c.ch == nil {
		return nil
	}
	return c.ch.Progress
}

// Update implements game.Scene.
func (c *Choices) Update(ctx *game.Context) error {
	if !ctx.Link.Linked() {
		ctx.Replace(NewAccount(ctx))
		return nil
	}
	if c.out != nil {
		select {
		case r := <-c.out:
			c.out = nil
			c.answered(ctx, r)
		default:
			switch {
			case c.step == chLoading && input.Back():
				// Only fetching: nothing is lost by going.
				c.leave(ctx)
			case c.step == chBusy && input.Back():
				// A change is on its way: going now would leave the
				// learner not knowing whether it happened.
				ctx.Sound.Play(audio.Wrong)
				c.still = true
			}
			if c.step == chLoading || c.step == chBusy {
				return nil
			}
		}
	}
	switch c.step {
	case chFailed:
		if input.Back() || input.Confirm() {
			c.leave(ctx)
		}
	case chOnPage:
		c.updatePage(ctx)
	case chStop:
		switch {
		case typedYes(ctx):
			ctx.Sound.Play(audio.Select)
			c.run(ctx, "Done. Your progress isn't being kept now. Your grown-ups get an email.",
				func(bg context.Context, lc *link.Client) error { return lc.StopProgress(bg) })
		case typedNo(ctx) || input.Back():
			ctx.Sound.Play(audio.Back)
			c.step = chList
		}
	case chAskStop:
		switch {
		case typedYes(ctx):
			ctx.Sound.Play(audio.Select)
			c.runWith(ctx, "", func(bg context.Context, lc *link.Client) (string, error) {
				already, err := lc.AskStopProgress(bg)
				if err == nil && !already.IsZero() {
					// A stale screen: nobody was emailed again.
					return "You already asked your grown-up on " + link.AskDate(already) + ".", nil
				}
				return "Done. We emailed your grown-up. Nothing changes until they decide.", err
			})
		case typedNo(ctx) || input.Back():
			ctx.Sound.Play(audio.Back)
			c.step = chList
		}
	case chWithdraw:
		switch {
		case typedYes(ctx):
			ctx.Sound.Play(audio.Select)
			purpose := c.ch.Visibility.Asks[c.items()[c.sel].ask].Purpose
			c.run(ctx, "Done. Your answer is No now. You can say yes again here at any time.", func(bg context.Context, lc *link.Client) error {
				return lc.SetAgreement(bg, purpose, false, "")
			})
		case typedNo(ctx) || input.Back():
			ctx.Sound.Play(audio.Back)
			c.step = chList
		}
	case chList:
		c.updateList(ctx)
	}
	return nil
}

// answered takes the answer to a request.
func (c *Choices) answered(ctx *game.Context, r chResult) {
	c.still = false
	switch {
	case r.err == nil:
		if r.done != "" {
			ctx.Sound.Play(audio.Perfect)
			c.say(r.done, pal.Lime)
		}
		sel := c.sel
		c.loaded(ctx, r.ch)
		c.sel = min(sel, len(c.items())-1)
	case r.acted:
		// The change worked; only showing the choices again didn't. Say
		// both, apart: the list as it was would be out of date, so it
		// isn't shown.
		ctx.Sound.Play(audio.Perfect)
		c.step = chFailed
		c.say(r.done, pal.Lime)
		c.msg2 = "But the choices couldn't be loaded again: " + lowerFirst(choicesProblem(r.err)) +
			" Open My choices again to see them."
	case c.ch == nil:
		c.step = chFailed
		c.say(choicesProblem(r.err), pal.Tan)
	case errors.Is(r.err, link.ErrNoticeChanged):
		// New words: fetch them, and the learner reads them again.
		ctx.Sound.Play(audio.Wrong)
		c.load(ctx, "")
		c.say("The words changed. Please read them again.", pal.Tan)
	default:
		ctx.Sound.Play(audio.Wrong)
		c.step = chList
		c.say(choicesProblem(r.err), pal.Tan)
	}
}

// choicesProblem says why My choices didn't work.
func choicesProblem(err error) string {
	switch {
	case errors.Is(err, link.ErrNoChoices):
		return "Your choices aren't on this website yet. Ask your grown-up."
	case errors.Is(err, link.ErrNotOwn):
		return "You can't change that yourself right now."
	case errors.Is(err, link.ErrAskedTooOften):
		return "You've asked a few times already. You can ask again later."
	}
	return upperFirst(link.Explain(err)) + "."
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func (c *Choices) updatePage(ctx *game.Context) {
	p := c.page
	switch {
	case input.Up():
		c.scroll = max(0, c.scroll-1)
		return
	case input.Down():
		c.scroll++
		return
	}
	switch p.kind {
	case chRead:
		if input.Confirm() || input.Back() {
			ctx.Sound.Play(audio.Back)
			c.step = chList
		}
	case chChild:
		if input.Confirm() {
			// "I've read it" lets a link by QR code sync; Esc keeps it
			// held (account_qr.go).
			noticeSeen(ctx)
		}
		switch {
		case input.Confirm():
			// Enter is "I've read it": recorded on the server, in the
			// background, while the next page or the list shows. A
			// failure is told again next time.
			ctx.Sound.Play(audio.Select)
			ref := p.ref
			go func(lc *link.Client) {
				bg, cancel := context.WithTimeout(context.Background(), askTimeout)
				defer cancel()
				lc.SawNotice(bg, ref)
			}(ctx.Link)
			c.say("", pal.Steel)
			c.next()
		case input.Back():
			// Esc isn't "I've read it" (PLAN §14, 2026-10-12): the
			// notice isn't recorded and comes again next time.
			ctx.Sound.Play(audio.Back)
			c.say("", pal.Steel)
			c.next()
		}
	case chOlder:
		if !(input.Confirm() || input.Back()) {
			return
		}
		// The server counted it as told when it sent it (once means
		// once), so Esc and Enter are the same. MarkOlder is for a
		// server that still waits for it.
		ctx.Sound.Play(audio.Select)
		go func(lc *link.Client) {
			bg, cancel := context.WithTimeout(context.Background(), askTimeout)
			defer cancel()
			lc.MarkOlder(bg)
		}(ctx.Link)
		c.ch.OlderNotice = false
		c.say("", pal.Steel)
		c.next()
	case chAgree, chRestart:
		switch {
		case typedYes(ctx):
			ctx.Sound.Play(audio.Select)
			ref, purpose := p.ref, p.purpose
			if p.kind == chAgree {
				c.run(ctx, "Done. You can change your mind here at any time.", func(bg context.Context, lc *link.Client) error {
					return lc.SetAgreement(bg, purpose, true, ref)
				})
			} else {
				c.run(ctx, "Asked. Your grown-up gets an email to say yes.", func(bg context.Context, lc *link.Client) error {
					return lc.StartProgress(bg, ref)
				})
			}
		case typedNo(ctx) || input.Back():
			ctx.Sound.Play(audio.Back)
			c.step = chList
		}
	}
}

func (c *Choices) updateList(ctx *game.Context) {
	items := c.items()
	c.sel = min(c.sel, len(items)-1)
	n := len(items)
	switch {
	case input.Back():
		c.leave(ctx)
	case input.Up():
		ctx.Sound.Play(audio.Blip)
		c.sel = (c.sel + n - 1) % n
	case input.Down():
		ctx.Sound.Play(audio.Blip)
		c.sel = (c.sel + 1) % n
	case input.Confirm() || input.Pressed(ebiten.KeySpace):
		ctx.Sound.Play(audio.Select)
		c.say("", pal.Steel)
		it := items[c.sel]
		switch it.kind {
		case chItemNotice:
			kind := chRead
			if ctx.Link.ChildNoticeDue() {
				kind = chChild // not recorded yet: Enter records it
			}
			c.page, c.scroll, c.step = chPage{kind: kind, title: c.ch.ChildNotice.Title, body: c.ch.ChildNotice.Body,
				ref: c.ch.ChildNotice.Ref}, 0, chOnPage
		case chItemAsk:
			v := c.ch.Visibility
			a := v.Asks[it.ask]
			if a.Agreed {
				c.step = chWithdraw
				break
			}
			c.page, c.scroll, c.step = chPage{kind: chAgree, title: v.Title, body: v.Body, ref: v.Ref,
				purpose: a.Purpose, label: a.Label}, 0, chOnPage
		case chItemStop:
			c.step = chStop
		case chItemRestart:
			r := c.progress().Restart
			c.page, c.scroll, c.step = chPage{kind: chRestart, title: r.Title, body: r.Body, ref: r.Ref, label: r.Label}, 0, chOnPage
		case chItemCancel:
			c.run(ctx, "Done. Your progress still isn't being kept.", func(bg context.Context, lc *link.Client) error {
				return lc.CancelProgress(bg)
			})
		case chItemAskStop:
			c.step = chAskStop
		case chItemTakeBack:
			c.run(ctx, "Done. You took back your request. Nothing has changed.", func(bg context.Context, lc *link.Client) error {
				return lc.TakeBackAsk(bg)
			})
		case chItemBack:
			c.leave(ctx)
		}
	}
}

// leave goes back to the Account screen, which doesn't open My choices
// again for a notice still being recorded.
func (c *Choices) leave(ctx *game.Context) {
	ctx.Sound.Play(audio.Back)
	ctx.Replace(&Account{bg: backdrop(11, 1.2), checked: true})
}

// Draw implements game.Scene.
func (c *Choices) Draw(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	gfx.DrawArt(dst, c.bg, 0, 0)
	f.DrawCentered(dst, "My choices", cx, 4, 3, pal.Yellow)
	const x, w = 32, game.ScreenW - 64
	switch c.step {
	case chLoading:
		f.DrawCentered(dst, "Asking the website…", cx, 160, 1, pal.Ice)
		f.DrawShadow(dst, "Esc back", 8, game.ScreenH-20, 1, pal.Ash)
		return
	case chFailed:
		y := 120
		for _, l := range wrapHard(f, c.msg, w-32) {
			f.DrawCentered(dst, l, cx, y, 1, c.msgCol)
			y += 18
		}
		if c.msg2 != "" {
			y += 12
			for _, l := range wrapHard(f, c.msg2, w-32) {
				f.DrawCentered(dst, l, cx, y, 1, pal.Tan)
				y += 18
			}
		}
		f.DrawShadow(dst, "Esc back", 8, game.ScreenH-20, 1, pal.Ash)
		return
	case chOnPage:
		c.drawPage(dst, ctx, x, w)
		return
	}
	c.drawList(dst, ctx, x, w)
	switch c.step {
	case chStop:
		const dw = 540
		dx, dy := dialog(dst, ctx, "STOP KEEPING MY PROGRESS?", dw, 250)
		ty := dy
		for _, p := range []struct {
			s   string
			col color.RGBA
		}{
			{"Halpwords will delete the progress it keeps for you on the website. Deleted progress can't be brought back.", pal.Tan},
			{"Your grown-ups get an email to tell them.", pal.Ice},
			{"This game's own progress stays here on this computer, and the game keeps working.", pal.Ice},
			{"Only you can ask to keep it again, and then a grown-up has to say yes.", pal.Ice},
		} {
			for _, l := range wrapHard(f, p.s, dw-40) {
				f.DrawShadow(dst, l, dx, ty, 1, p.col)
				ty += 16
			}
			ty += 4
		}
		f.DrawShadow(dst, "Y stop keeping it   N cancel", dx, dy+160, 1, pal.Ash)
	case chAskStop:
		const dw = 480
		dx, dy := dialog(dst, ctx, "ASK YOUR GROWN-UP?", dw, 190)
		ty := dy
		for _, p := range []struct {
			s   string
			col color.RGBA
		}{
			{"Do you want to ask your grown-up to stop keeping your progress?", pal.Ice},
			{"We will send them an email. If they say yes, Halpwords deletes your progress. Your games still work.", pal.Ice},
			{"Nothing changes until they decide.", pal.Ice},
		} {
			for _, l := range wrapHard(f, p.s, dw-40) {
				f.DrawShadow(dst, l, dx, ty, 1, p.col)
				ty += 16
			}
			ty += 4
		}
		f.DrawShadow(dst, "Y yes, ask them   N no", dx, dy+160, 1, pal.Ash)
	case chWithdraw:
		const dw = 480
		a := c.ch.Visibility.Asks[c.items()[c.sel].ask]
		dx, dy := dialog(dst, ctx, "SAY NO?", dw, 190)
		ty := dy
		for _, l := range wrapHard(f, a.Label+": your answer changes to No, so this stops for you. "+
			"You can say yes again here at any time.", dw-40) {
			f.DrawShadow(dst, l, dx, ty, 1, pal.Ice)
			ty += 16
		}
		f.DrawShadow(dst, "Y say no   N keep yes", dx, dy+104, 1, pal.Ash)
	case chBusy:
		dx, dy := dialog(dst, ctx, "JUST A MOMENT", 380, 110)
		f.DrawShadow(dst, "Asking the website…", dx, dy, 1, pal.Ice)
		if c.still {
			f.DrawShadow(dst, "Still sending. Wait for the answer.", dx, dy+24, 1, pal.Tan)
		}
	}
}

// wrapHard is wrap that also breaks a word too wide for a line, such as
// a web address, so no line runs off the screen.
func wrapHard(f *gfx.Font, s string, width int) []string {
	var lines []string
	for _, l := range wrap(f, s, width) {
		for f.Width(l, 1) > width {
			r := []rune(l)
			n := len(r) - 1
			for n > 1 && f.Width(string(r[:n]), 1) > width {
				n--
			}
			lines = append(lines, string(r[:n]))
			l = strings.TrimLeft(string(r[n:]), " ")
		}
		lines = append(lines, l)
	}
	return lines
}

// pageLines are a notice's lines, wrapped to width, with a gap between
// paragraphs.
func pageLines(f *gfx.Font, body []string, width int) []string {
	var lines []string
	for i, p := range body {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, wrapHard(f, p, width)...)
	}
	return lines
}

// The page's layout: the window, and where its text ends.
const (
	pageTop    = 34
	pageBottom = 284 // the last line of text starts here at most
)

// pageRows is how many lines of a notice fit below a title of n lines.
func pageRows(titleLines int) int { return (pageBottom-pageBodyY(titleLines))/18 + 1 }

// pageBodyY is where a notice's text starts below a title of n lines.
func pageBodyY(titleLines int) int { return pageTop + 32 + 18*(titleLines-1) }

// pageTitle is the title's lines: at most two, the second cut short.
func pageTitle(f *gfx.Font, title string, width int) []string {
	t := wrapHard(f, title, width)
	if len(t) > 2 {
		t = []string{t[0], fit(f, strings.Join(t[1:], " "), width, 1)}
	}
	return t
}

func (c *Choices) drawPage(dst *ebiten.Image, ctx *game.Context, x, w int) {
	f := ctx.Font
	p := c.page
	gfx.Window(dst, x, pageTop, w, 280)
	title := pageTitle(f, p.title, w-32)
	for i, l := range title {
		f.DrawCentered(dst, l, game.ScreenW/2, pageTop+10+i*18, 1, pal.Yellow)
	}
	by := pageBodyY(len(title))
	rows := pageRows(len(title))
	lines := pageLines(f, p.body, w-32)
	c.scroll = min(c.scroll, max(0, len(lines)-rows))
	for i := 0; i < rows && c.scroll+i < len(lines); i++ {
		f.DrawShadow(dst, lines[c.scroll+i], x+16, by+i*18, 1, pal.Ice)
	}
	more := ""
	if len(lines) > rows {
		// Where the reader is: the last line shown of how many, or the
		// end.
		at := fmt.Sprintf("%d/%d", c.scroll+rows, len(lines))
		if c.scroll+rows >= len(lines) {
			at = "end"
		}
		more = "↑/↓ read more (" + at + ")   "
	}
	switch p.kind {
	case chAgree, chRestart:
		f.DrawShadow(dst, fit(f, "Y: "+p.label, w, 1), x, game.ScreenH-40, 1, pal.Lime)
		f.DrawShadow(dst, more+"Y yes   N not now", 8, game.ScreenH-20, 1, pal.Ash)
	default:
		if c.msg != "" {
			f.DrawShadow(dst, fit(f, c.msg, w, 1), x, game.ScreenH-40, 1, c.msgCol)
		}
		keys := "Enter OK"
		if p.kind == chChild {
			keys = "Enter I've read it   Esc later"
		}
		f.DrawShadow(dst, more+keys, 8, game.ScreenH-20, 1, pal.Ash)
	}
}

// infoLines are what the list's window says, wrapped to width.
func (c *Choices) infoLines(f *gfx.Font, st link.Status, width int) []chLine {
	var out []chLine
	line := func(s string, col color.RGBA) {
		for _, l := range wrapHard(f, s, width) {
			out = append(out, chLine{l, col})
		}
	}
	p := c.progress()
	stopped := st.Stopped || p != nil && p.Stopped
	if !stopped {
		// While stopped nothing is kept, so nobody sees it: saying who
		// can would mislead.
		line(link.SeenByText(st.SeenBy), pal.Ice)
	}
	switch {
	case c.ch == nil:
	case !c.ch.Family:
		// As child-link/1 says (halpwords-server, 2026-10-12).
		line("Your teacher looks after what Halpwords keeps, and your school if your class belongs to one. There's nothing for you to choose here.", pal.Steel)
	case p != nil && p.Waiting:
		line("Your progress isn't being kept. You asked to keep it again: your grown-up has to say yes by email.", pal.Steel)
	case p != nil && p.Stopped && p.Restart == nil:
		line("Your progress isn't being kept. This game still works on this computer. To keep it again, ask your parent or guardian.", pal.Steel)
	case p != nil && p.Stopped:
		line("Your progress isn't being kept. This game still works on this computer.", pal.Steel)
	case p != nil && p.CanStop:
		line("Halpwords keeps your progress. You can stop that yourself.", pal.Steel)
	case p != nil && p.Ask != nil && !p.Ask.WaitingSince.IsZero():
		line("You asked your grown-up on "+link.AskDate(p.Ask.WaitingSince)+
			" to stop keeping your progress. Nothing changes until they decide. If they don't answer in 14 days, your request ends and you can ask again.", pal.Steel)
	case p != nil && p.Ask != nil && p.Ask.Kept:
		s := "Your grown-up decided to keep your progress. You can talk to them about it."
		if !p.Ask.AskAgainAfter.IsZero() {
			s += " You can ask again after " + link.AskDate(p.Ask.AskAgainAfter) + "."
		}
		line(s, pal.Steel)
	case p != nil && p.Ask != nil && p.Ask.CanAsk:
		line("Halpwords keeps your progress, like the words you know and your scores, so your games can save it. If you want that to stop, you can ask your grown-up. They decide. "+
			"If they say yes, Halpwords deletes your progress. Your games still work.", pal.Steel)
	case p != nil && p.Ask != nil:
		// Nobody to email now (as the website's kid.ask.talk).
		line("Halpwords keeps your progress. If you want that to stop, talk to your grown-up about it.", pal.Steel)
	case c.ch.Visibility == nil:
		line("For now, your grown-up makes these choices for you.", pal.Steel)
	}
	if c.ch != nil && c.ch.Family && c.ch.Visibility != nil && len(c.ch.Visibility.Asks) == 0 {
		// As the website's kid.choices.empty (and nothing_shown when
		// there is a progress line) say.
		if p != nil {
			line("Nobody outside your family can see you on Halpwords.", pal.Steel)
		} else {
			line("Nobody outside your family can see you on Halpwords. If your parent or guardian wants to change that, you decide here too.", pal.Steel)
		}
	}
	if len(out) > chInfoRows {
		out = out[:chInfoRows]
		out[chInfoRows-1].s = fit(f, out[chInfoRows-1].s+"…", width, 1)
	}
	return out
}

type chLine struct {
	s   string
	col color.RGBA
}

// The list's layout: at most chInfoRows lines above it, its items end
// above chListEnd, and the selected item's hint goes below them, above the
// message.
const (
	chInfoRows = 6
	chListEnd  = 256
	chRow      = 20
	chHintY    = 262
)

func (c *Choices) drawList(dst *ebiten.Image, ctx *game.Context, x, w int) {
	f := ctx.Font
	info := c.infoLines(f, ctx.Link.Status(), w-32)
	gfx.Window(dst, x, 40, w, 20+len(info)*18)
	for i, l := range info {
		f.DrawShadow(dst, l.s, x+16, 52+i*18, 1, l.col)
	}

	items := c.items()
	my := 40 + 20 + len(info)*18 + 10
	rows := max(3, (chListEnd-my)/chRow)
	// The list scrolls to keep the chosen item in view.
	c.top = min(c.top, c.sel)
	if c.sel >= c.top+rows {
		c.top = c.sel - rows + 1
	}
	c.top = max(0, min(c.top, len(items)-rows))
	for i := c.top; i < len(items) && i < c.top+rows; i++ {
		it := items[i]
		col := pal.Steel
		iy := my + (i-c.top)*chRow
		if i == c.sel && c.step == chList {
			col = pal.White
			if ctx.Tick/20%2 == 0 {
				f.Draw(dst, "►", x+16, iy, 1, pal.Yellow)
			}
		}
		label := ""
		switch it.kind {
		case chItemNotice:
			label = "Who can see your progress"
		case chItemAsk:
			a := c.ch.Visibility.Asks[it.ask]
			yes, ycol := "No", pal.Tan
			if a.Agreed {
				yes, ycol = "Yes", pal.Lime
			}
			f.DrawShadow(dst, yes, x+w-16-f.Width(yes, 1), iy, 1, ycol)
			label = fit(f, a.Label, w-120, 1)
		case chItemStop:
			label = "Stop keeping my progress"
		case chItemRestart:
			label = fit(f, c.progress().Restart.Label, w-80, 1)
		case chItemCancel:
			label = "Don't ask to keep it after all"
		case chItemAskStop:
			label = fit(f, "Ask my grown-up to stop keeping my progress", w-80, 1)
		case chItemTakeBack:
			label = "Take back my request"
		case chItemBack:
			label = "Back"
		}
		f.DrawShadow(dst, label, x+36, iy, 1, col)
		switch {
		case i == c.top && c.top > 0:
			f.Draw(dst, "↑", x+w-8, iy, 1, pal.Ash)
		case i == c.top+rows-1 && i < len(items)-1:
			f.Draw(dst, "↓", x+w-8, iy, 1, pal.Ash)
		}
	}

	// The chosen item's hint, from the server.
	keys := "↑/↓ choose   Enter select   Esc back"
	if c.sel < len(items) {
		hint := ""
		switch it := items[c.sel]; it.kind {
		case chItemAsk:
			hint = c.ch.Visibility.Asks[it.ask].Hint
			keys = "↑/↓ choose   Enter change   Esc back"
		case chItemRestart:
			hint = c.progress().Restart.Hint
		}
		if hint != "" {
			h := wrapHard(f, hint, w)
			if len(h) > 2 {
				h = []string{h[0], fit(f, strings.Join(h[1:], " "), w, 1)}
			}
			for i, l := range h {
				f.DrawShadow(dst, l, x, chHintY+i*18, 1, pal.Steel)
			}
		}
	}
	if c.msg != "" {
		lines := msgLines(f, c.msg)
		for i, l := range lines {
			f.DrawCentered(dst, l, game.ScreenW/2, game.ScreenH-44-(len(lines)-1-i)*18, 1, c.msgCol)
		}
	}
	f.DrawShadow(dst, keys, 8, game.ScreenH-20, 1, pal.Ash)
}
