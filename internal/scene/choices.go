package scene

import (
	"context"
	"errors"
	"image/color"

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
// their progress. Only what applies is shown. The words of the notices
// come from the server, so they are the ones the website shows.

// chStep is what My choices is doing.
type chStep int

const (
	chLoading chStep = iota
	chFailed         // couldn't load: msg says why
	chOnPage         // showing a notice
	chList           // the choices
	chStop           // confirming a stop
	chBusy           // an answer is on its way
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
	chItemBack
)

// chResult is the answer to a request made in the background.
type chResult struct {
	ch   *link.Choices
	err  error
	done string // what to say when it worked
}

// Choices is the My choices screen.
type Choices struct {
	bg     *ebiten.Image
	step   chStep
	sel    int
	scroll int
	ch     *link.Choices
	// pages are notices to show before the list: the child notice, the
	// one at 16 or 17.
	pages  []chPage
	page   chPage
	out    chan chResult
	msg    string
	msgCol color.RGBA
}

// NewChoices opens My choices and fetches them.
func NewChoices(ctx *game.Context) game.Scene {
	c := &Choices{bg: backdrop(11, 1.2)}
	c.load(ctx, "")
	return c
}

// newChoicesWith opens My choices with choices already fetched, such as
// the Account screen's, to tell what is due.
func newChoicesWith(ctx *game.Context, ch *link.Choices) *Choices {
	c := &Choices{bg: backdrop(11, 1.2)}
	c.loaded(ctx, ch)
	return c
}

func (c *Choices) say(msg string, col color.RGBA) { c.msg, c.msgCol = msg, col }

// run makes a request in the background, then fetches the choices again.
func (c *Choices) run(ctx *game.Context, done string, do func(context.Context, *link.Client) error) {
	lc := ctx.Link
	out := make(chan chResult, 1)
	c.out, c.step = out, chBusy
	go func() {
		bg, cancel := context.WithTimeout(context.Background(), askTimeout)
		defer cancel()
		if do != nil {
			if err := do(bg, lc); err != nil {
				out <- chResult{err: err}
				return
			}
		}
		ch, err := lc.Choices(bg)
		out <- chResult{ch: ch, err: err, done: done}
	}()
}

func (c *Choices) load(ctx *game.Context, done string) {
	c.run(ctx, done, nil)
	c.step = chLoading
}

// loaded takes fetched choices: the notices due first, then the list.
func (c *Choices) loaded(ctx *game.Context, ch *link.Choices) {
	c.ch, c.pages = ch, nil
	if ctx.Link.ChildNoticeDue() && ch.ChildNotice.Ref != "" {
		c.pages = append(c.pages, chPage{kind: chChild, title: ch.ChildNotice.Title, body: ch.ChildNotice.Body,
			ref: ch.ChildNotice.Ref})
	}
	if ch.OlderNotice {
		c.pages = append(c.pages, chPage{kind: chOlder, title: "You can decide more for yourself now",
			body: []string{olderText(ch)}})
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
			items = append(items, chItem{kind: chItemRestart})
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
			if c.step == chLoading || c.step == chBusy {
				if input.Back() {
					c.leave(ctx)
				}
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
		case input.Pressed(ebiten.KeyY):
			ctx.Sound.Play(audio.Select)
			c.run(ctx, "Done. Your progress isn't being kept now. Your grown-ups get an email.",
				func(bg context.Context, lc *link.Client) error { return lc.StopProgress(bg) })
		case input.Pressed(ebiten.KeyN) || input.Back():
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
	switch {
	case r.err == nil:
		if r.done != "" {
			ctx.Sound.Play(audio.Perfect)
			c.say(r.done, pal.Lime)
		}
		sel := c.sel
		c.loaded(ctx, r.ch)
		c.sel = min(sel, len(c.items())-1)
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
	}
	return upperFirst(link.Explain(err)) + "."
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
	case chChild, chOlder:
		if !(input.Confirm() || input.Back()) {
			return
		}
		ctx.Sound.Play(audio.Select)
		ref := p.ref
		mark := func(bg context.Context, lc *link.Client) error { return lc.MarkOlder(bg) }
		if p.kind == chChild {
			mark = func(bg context.Context, lc *link.Client) error { return lc.SawNotice(bg, ref) }
		}
		// Told: the server keeps it, in the background, while the next
		// page or the list shows. A failure is told again next time.
		go func(lc *link.Client) {
			bg, cancel := context.WithTimeout(context.Background(), askTimeout)
			defer cancel()
			mark(bg, lc)
		}(ctx.Link)
		if p.kind == chOlder {
			c.ch.OlderNotice = false
		}
		c.next()
	case chAgree, chRestart:
		switch {
		case input.Pressed(ebiten.KeyY):
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
		case input.Pressed(ebiten.KeyN) || input.Back():
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
				purpose := a.Purpose
				c.run(ctx, "Done. You can say yes again here at any time.", func(bg context.Context, lc *link.Client) error {
					return lc.SetAgreement(bg, purpose, false, "")
				})
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
		for i, l := range wrap(f, c.msg, w-32) {
			f.DrawCentered(dst, l, cx, 140+i*18, 1, c.msgCol)
		}
		f.DrawShadow(dst, "Esc back", 8, game.ScreenH-20, 1, pal.Ash)
		return
	case chOnPage:
		c.drawPage(dst, ctx, x, w)
		return
	}
	c.drawList(dst, ctx, x, w)
	if c.step == chStop {
		dx, dy := dialog(dst, ctx, "STOP KEEPING MY PROGRESS?", 520, 210)
		for i, l := range []string{
			"Halpwords will delete the progress it keeps for you",
			"on the website, and email your grown-ups to tell them.",
			"This game keeps working on this computer, but it",
			"won't sync. Your grown-ups can't turn it back on:",
			"only you can, and then they have to say yes too.",
		} {
			f.DrawShadow(dst, l, dx, dy+i*16, 1, pal.Ice)
		}
		f.DrawShadow(dst, "Y stop keeping it   N cancel", dx, dy+100, 1, pal.Ash)
	}
	if c.step == chBusy {
		dx, dy := dialog(dst, ctx, "JUST A MOMENT", 360, 110)
		f.DrawShadow(dst, "Asking the website…", dx, dy, 1, pal.Ice)
		f.DrawShadow(dst, "Esc back", dx, dy+24, 1, pal.Ash)
	}
}

// pageLines are a notice's lines, wrapped to width, with a gap between
// paragraphs.
func pageLines(f *gfx.Font, body []string, width int) []string {
	var lines []string
	for i, p := range body {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, wrap(f, p, width)...)
	}
	return lines
}

// pageRows is how many lines of a notice fit on the screen.
const pageRows = 13

func (c *Choices) drawPage(dst *ebiten.Image, ctx *game.Context, x, w int) {
	f := ctx.Font
	p := c.page
	gfx.Window(dst, x, 34, w, 280)
	f.DrawCentered(dst, fit(f, p.title, w-32, 1), game.ScreenW/2, 44, 1, pal.Yellow)
	lines := pageLines(f, p.body, w-32)
	c.scroll = min(c.scroll, max(0, len(lines)-pageRows))
	for i := 0; i < pageRows && c.scroll+i < len(lines); i++ {
		f.DrawShadow(dst, lines[c.scroll+i], x+16, 66+i*18, 1, pal.Ice)
	}
	more := ""
	if len(lines) > pageRows {
		more = "↑/↓ read more   "
	}
	switch p.kind {
	case chAgree, chRestart:
		f.DrawShadow(dst, fit(f, "Y: "+p.label, w, 1), x, game.ScreenH-40, 1, pal.Lime)
		f.DrawShadow(dst, more+"Y yes   N not now", 8, game.ScreenH-20, 1, pal.Ash)
	default:
		f.DrawShadow(dst, more+"Enter OK", 8, game.ScreenH-20, 1, pal.Ash)
	}
}

func (c *Choices) drawList(dst *ebiten.Image, ctx *game.Context, x, w int) {
	f := ctx.Font
	gfx.Window(dst, x, 40, w, 104)
	ty := 52
	line := func(s string, col color.RGBA) {
		for _, l := range wrap(f, s, w-32) {
			f.DrawShadow(dst, l, x+16, ty, 1, col)
			ty += 18
		}
	}
	st := ctx.Link.Status()
	line(link.SeenByText(st.SeenBy), pal.Ice)
	p := c.progress()
	switch {
	case c.ch == nil:
	case !c.ch.Family:
		line("Your teacher or school looks after what Halpwords keeps. There's nothing for you to choose here.", pal.Steel)
	case p != nil && p.Waiting:
		line("Your progress isn't being kept. You asked to keep it again: your grown-up has to say yes by email.", pal.Steel)
	case p != nil && p.Stopped:
		line("Your progress isn't being kept. This game still works on this computer.", pal.Steel)
	case p != nil && p.CanStop:
		line("Halpwords keeps your progress. You can stop that yourself.", pal.Steel)
	case c.ch.Visibility == nil:
		line("For now, your grown-up makes these choices for you.", pal.Steel)
	}

	items := c.items()
	my := 156
	for i, it := range items {
		col := pal.Steel
		iy := my + i*22
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
			label = fit(f, a.Label, w-100, 1)
		case chItemStop:
			label = "Stop keeping my progress"
		case chItemRestart:
			label = fit(f, c.progress().Restart.Label, w-60, 1)
		case chItemCancel:
			label = "Don't ask to keep it after all"
		case chItemBack:
			label = "Back"
		}
		f.DrawShadow(dst, label, x+36, iy, 1, col)
	}
	if c.msg != "" {
		lines := msgLines(f, c.msg)
		for i, l := range lines {
			f.DrawCentered(dst, l, game.ScreenW/2, game.ScreenH-44-(len(lines)-1-i)*18, 1, c.msgCol)
		}
	}
	f.DrawShadow(dst, "↑/↓ choose   Enter select   Esc back", 8, game.ScreenH-20, 1, pal.Ash)
}
