package scene

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"strings"
	"time"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/pal"
)

// acMode is what the Account screen is doing.
type acMode int

const (
	acBrowse  acMode = iota
	acCode           // typing a pairing code
	acUnlink         // confirming an unlink
	acQR             // showing a QR code to link by (account_qr.go)
	acConfirm        // "Is this you?" after a link by QR code
)

// acItem is an entry in the Account screen's menu.
type acItem int

const (
	acLink acItem = iota
	acSync
	acUnlinkItem
	acBack
	acSchool
	acChoices
	acFinish // "Is this you?" again, after Esc on it (account_qr.go)
)

var acLabels = [...]string{"Link to a grown-up's account", "Sync now", "Unlink", "Back", "Sign in at school", "My choices", "Finish linking"}

// school reports whether the game signed in at school, with a login card,
// the class code or a school account: it signs out rather than unlinks
// (W2.5, W5.4).
func school(ctx *game.Context) bool {
	w := ctx.Link.Way()
	return w == link.WayCard || w == link.WayClass || w == link.WaySSO
}

// Account links the game to a grown-up's account on the website, shows who
// can see the player's progress, and syncs or unlinks.
type Account struct {
	bg   *ebiten.Image
	mode acMode
	sel  int
	code []rune // the pairing code being typed, without the hyphen
	// linking is set while a link started here is on its way.
	linking bool
	// signingOut reports when a sign-out has finished.
	signingOut func() bool
	syncUntil  int // Sync Now asked for: Status.Syncs says when it is done
	msg        string
	msgCol     color.RGBA
	// checked is set once My choices were fetched to see whether a
	// notice is due (the child notice after linking, the one at 16 or
	// 17), and checking while they are on their way. noChoices: the
	// server has no My choices.
	checked   bool
	checking  chan chResult
	noChoices bool
	// due are fetched choices with a notice due, waiting to open: on the
	// next key, which then only opens them, or after dueWait more ticks,
	// so "Linked!" is read and no key press lands on the wrong screen.
	due     *link.Choices
	dueWait int
	// linked is "Linked!", said again on the notice it opens.
	linked string
	// pair is the link by QR code on the screen (account_qr.go).
	pair *acPair
	// later: the child chose Esc on "Is this you?". The link stays held
	// and Finish linking asks again.
	later bool
}

// choicesPause is how long the Account screen waits for a key before it
// opens a notice that is due: about two seconds.
const choicesPause = 120

// NewAccount creates the Account screen.
func NewAccount(ctx *game.Context) game.Scene {
	return &Account{bg: backdrop(11, 1.2)}
}

func (a *Account) items(ctx *game.Context) []acItem {
	if ctx.Link.Status().Unconfirmed {
		// Held: nothing syncs until the child says who they are.
		return []acItem{acFinish, acUnlinkItem, acBack}
	}
	if ctx.Link.Linked() {
		var items []acItem
		if !ctx.Link.Status().Stopped {
			// Stopped, nothing syncs: no button that can't do anything.
			items = append(items, acSync)
		}
		if !a.noChoices {
			items = append(items, acChoices)
		}
		return append(items, acUnlinkItem, acBack)
	}
	return []acItem{acLink, acSchool, acBack}
}

func (a *Account) say(msg string, c color.RGBA) { a.msg, a.msgCol = msg, c }

// Update implements game.Scene.
func (a *Account) Update(ctx *game.Context) error {
	// Nobody chosen (a switch failed): the Account screen would link a
	// game that has nobody to link it to.
	if ctx.Learners != nil && ctx.Learner() == nil {
		ctx.Replace(NewLearners(ctx))
		return nil
	}
	if a.signingOut != nil {
		if a.signingOut() {
			a.signingOut = nil
			ctx.Replace(NewLearners(ctx))
		}
		return nil
	}
	st := ctx.Link.Status()
	switch {
	case a.mode == acQR:
		a.updatePair(ctx)
		return nil
	case a.mode == acConfirm:
		a.updateConfirm(ctx)
		return nil
	case st.Unconfirmed && !st.Signing && !a.later:
		// Linked by QR code, and the child hasn't said who they are:
		// that comes first, before My choices and any sync.
		a.openConfirm(ctx)
		return nil
	}
	if ch := a.checkChoices(ctx, st); ch != nil {
		ctx.Replace(ch)
		return nil
	}
	if a.linking && !st.Signing {
		a.linking = false
		switch {
		case st.Linked:
			ctx.Sound.Play(audio.Perfect)
			a.linked = "Linked! Your progress now goes to your grown-up's account."
			a.say(a.linked, pal.Lime)
			a.sel = 0
		default:
			ctx.Sound.Play(audio.Wrong)
			a.say(upperFirst(link.Explain(st.Err))+".", pal.Rose)
			a.mode = acCode
		}
	}
	if a.syncUntil > 0 && (st.Syncs >= a.syncUntil && !st.Busy || !st.Linked) {
		a.syncUntil = 0
		switch {
		case !st.Linked:
			a.say("", pal.Steel)
		case st.Suspended, st.Err != nil:
			// A problem says more than being stopped does.
			s, _ := syncProblem(st)
			a.say(s, pal.Rose)
		case st.Stopped:
			a.say("Your progress isn't being kept, so nothing syncs.", pal.Ice)
		default:
			a.say("Synced.", pal.Lime)
		}
	}
	switch a.mode {
	case acCode:
		a.updateCode(ctx)
		return nil
	case acUnlink:
		switch {
		case typedYes(ctx) && school(ctx):
			ctx.Sound.Play(audio.Select)
			a.signingOut = ctx.SignOut()
			a.mode = acBrowse
			a.say("Signing out…", pal.Ice)
		case typedYes(ctx):
			ctx.Link.Unlink()
			ctx.Sound.Play(audio.Select)
			a.say("Unlinked. Your progress is still here.", pal.Lime)
			a.mode, a.sel = acBrowse, 0
		case typedNo(ctx) || input.Back():
			ctx.Sound.Play(audio.Back)
			a.mode = acBrowse
		}
		return nil
	}
	items := a.items(ctx)
	a.sel = min(a.sel, len(items)-1)
	n := len(items)
	switch {
	case input.Back():
		a.leave(ctx)
	case input.Up():
		ctx.Sound.Play(audio.Blip)
		a.sel = (a.sel + n - 1) % n
	case input.Down():
		ctx.Sound.Play(audio.Blip)
		a.sel = (a.sel + 1) % n
	case input.Confirm() || input.Pressed(ebiten.KeySpace):
		switch items[a.sel] {
		case acLink:
			if a.linking {
				return nil
			}
			ctx.Sound.Play(audio.Select)
			a.openPair(ctx)
		case acFinish:
			ctx.Sound.Play(audio.Select)
			a.later = false
			a.openConfirm(ctx)
		case acSync:
			ctx.Sound.Play(audio.Select)
			st := ctx.Link.Status()
			// A sync already running ends before the one asked for
			// starts.
			a.syncUntil = st.Syncs + 1
			if st.Busy {
				a.syncUntil++
			}
			ctx.Link.SyncNow()
			a.say("Syncing…", pal.Ice)
		case acChoices:
			ctx.Sound.Play(audio.Select)
			ctx.Replace(NewChoices(ctx))
		case acUnlinkItem:
			ctx.Sound.Play(audio.Select)
			a.mode = acUnlink
		case acSchool:
			ctx.Sound.Play(audio.Select)
			// A school sign-in never joins a folder that has another
			// learner's progress: it gets a learner of its own.
			prev := ""
			if l := ctx.Learner(); l != nil && ctx.Learners != nil && !ctx.Pristine(l) {
				prev = ctx.Learners.Current
				if _, err := ctx.AddLearner(""); err != nil {
					ctx.Sound.Play(audio.Wrong)
					a.say("Couldn't add a learner: "+err.Error(), pal.Rose)
					break
				}
			}
			ctx.Replace(NewSignIn(ctx, prev))
		case acBack:
			a.leave(ctx)
		}
	}
	return nil
}

// checkChoices fetches My choices once the game is linked, in the
// background, to open them when a notice is due: the child notice
// after linking, or the one-time notice at 16 or 17. It returns the
// screen to open, or nil. It opens them on the next key, which does
// nothing else, or after a pause, so "Linked!" isn't cut off; the
// notice says it again. The choices it fetched are the ones shown: the
// server counted the notice at 16 or 17 as told when it sent it.
func (a *Account) checkChoices(ctx *game.Context, st link.Status) *Choices {
	if !st.Linked {
		a.checked, a.checking, a.noChoices, a.due, a.linked = false, nil, false, nil, ""
		return nil
	}
	if a.due != nil {
		if a.mode != acBrowse {
			return nil
		}
		a.dueWait--
		if a.dueWait > 0 && !anyKey(ctx) {
			return nil
		}
		ch, msg := a.due, a.linked
		a.due, a.linked = nil, ""
		return newChoicesWith(ctx, ch, msg)
	}
	if a.checking != nil {
		select {
		case r := <-a.checking:
			a.checking = nil
			a.noChoices = errors.Is(r.err, link.ErrNoChoices)
			if r.err == nil && (ctx.Link.ChildNoticeDue() && r.ch.ChildNotice.Ref != "" || r.ch.OlderNotice) {
				a.due, a.dueWait = r.ch, choicesPause
			}
		default:
		}
		return nil
	}
	if a.checked || a.linking || st.Signing {
		return nil
	}
	a.checked = true
	out := make(chan chResult, 1)
	a.checking = out
	go func(lc *link.Client) {
		bg, cancel := context.WithTimeout(context.Background(), askTimeout)
		defer cancel()
		ch, err := lc.Choices(bg)
		out <- chResult{ch: ch, err: err}
	}(ctx.Link)
	return nil
}

// anyKey reports whether a key the menus use was pressed, or a
// character typed, this tick.
func anyKey(ctx *game.Context) bool {
	return input.Confirm() || input.Back() || input.Up() || input.Down() || input.Pressed(ebiten.KeySpace) ||
		ctx.Input != nil && len(ctx.Input.Chars) > 0
}

func (a *Account) leave(ctx *game.Context) {
	ctx.Link.ClearNote()
	ctx.Sound.Play(audio.Back)
	ctx.Replace(NewTitle(ctx))
}

// codeLen is how many characters a pairing code has, without the hyphen.
const codeLen = 8

func (a *Account) updateCode(ctx *game.Context) {
	if a.linking {
		return
	}
	for _, r := range ctx.Input.Chars {
		r = unicode.ToUpper(r)
		if (r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') && len(a.code) < codeLen {
			a.code = append(a.code, r)
			ctx.Sound.Play(audio.Key)
		}
	}
	switch {
	case input.Repeat(ebiten.KeyBackspace) && len(a.code) > 0:
		a.code = a.code[:len(a.code)-1]
		ctx.Sound.Play(audio.Erase)
	case input.Back():
		ctx.Sound.Play(audio.Back)
		a.mode = acBrowse
		a.say("", pal.Steel)
	case input.Confirm() && len(a.code) == codeLen:
		ctx.Sound.Play(audio.Select)
		ctx.Link.Link(string(a.code))
		a.linking = true
		a.mode = acBrowse
		a.say("Linking…", pal.Ice)
	case input.Confirm():
		ctx.Sound.Play(audio.Wrong)
		a.say(fmt.Sprintf("The code has %d letters and numbers.", codeLen), pal.Rose)
	}
}

// showCode puts the hyphen in a pairing code: ABCD-EFGH.
func showCode(code []rune) string {
	s := string(code) + strings.Repeat("_", codeLen-len(code))
	return s[:4] + "-" + s[4:]
}

// ago says how long ago t was, in a few words.
func ago(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case t.IsZero():
		return "not yet"
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d/time.Minute), "minute") + " ago"
	case d < 48*time.Hour:
		return plural(int(d/time.Hour), "hour") + " ago"
	}
	return plural(int(d/(24*time.Hour)), "day") + " ago"
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// msgLines wraps the Account screen's message to fit the screen, in at
// most two lines; a longer one is cut short.
func msgLines(f *gfx.Font, msg string) []string {
	const w = game.ScreenW - 32
	lines := wrap(f, msg, w)
	if len(lines) > 2 {
		lines = []string{lines[0], strings.Join(lines[1:], " ")}
	}
	lines[len(lines)-1] = fit(f, lines[len(lines)-1], w, 1)
	return lines
}

// windowLines wraps s at spaces into the lines of the Account window,
// width px wide; the last is cut short if it can't be wrapped.
func windowLines(f *gfx.Font, s string, width int) []string {
	var out []string
	for f.Width(s, 1) > width {
		cut := strings.LastIndex(s[:min(len(s), width/8)], " ")
		if cut <= 0 {
			break
		}
		out = append(out, s[:cut])
		s = s[cut+1:]
	}
	return append(out, fit(f, s, width, 1))
}

// Draw implements game.Scene.
func (a *Account) Draw(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	gfx.DrawArt(dst, a.bg, 0, 0)
	f.DrawCentered(dst, "Account", cx, 4, 3, pal.Yellow)

	st := ctx.Link.Status()
	const x, w = 40, game.ScreenW - 80
	y := 50
	gfx.Window(dst, x, y, w, 156)
	tx := x + 16
	ty := y + 12
	line := func(s string, c color.RGBA) {
		for _, l := range windowLines(f, s, w-32) {
			f.DrawShadow(dst, l, tx, ty, 1, c)
			ty += 18
		}
	}
	if st.Unconfirmed {
		line("A grown-up linked this game to their account.", pal.Yellow)
		line("Nothing is sent until you say it's you.", pal.Ice)
		line("Choose Finish linking to check who it's linked to.", pal.Ice)
		line("The game plays the same meanwhile.", pal.Steel)
	} else if st.Linked {
		name := st.Learner
		if name == "" {
			name = "a learner"
		}
		line("Linked to "+name+"'s account.", pal.Lime)
		if !st.Stopped {
			// Nothing is kept while stopped, so nobody sees it.
			line(link.SeenByText(st.SeenBy), pal.Ice)
		}
		line("Last sync: "+ago(time.Now(), st.LastSync), pal.Steel)
		switch {
		case st.Stopped:
			// syncProblem says so.
		case st.Busy:
			line("Syncing…", pal.Ice)
		case st.Pending > 0:
			line(plural(st.Pending, "update")+" waiting to be sent", pal.Steel)
		default:
			line("Everything is sent.", pal.Steel)
		}
		if st.Lists > 0 {
			line(plural(st.Lists, "assigned list")+", in Word Lists", pal.Steel)
		}
		if s, c := syncProblem(st); s != "" {
			line(s, c)
		}
		if st.Outdated {
			line("A new version of Halpwords is out: please update.", pal.Yellow)
		}
	} else {
		if st.Note != "" {
			line(st.Note, pal.Yellow)
		}
		line("Link this game to a parent's or teacher's account on", pal.Ice)
		line("the Halpwords website. They can then see your progress", pal.Ice)
		line("and give you word lists. Ask them for a code.", pal.Ice)
		line("The game plays the same without it.", pal.Steel)
	}

	// The menu.
	items := a.items(ctx)
	my := 214
	for i, it := range items {
		c := pal.Steel
		iy := my + i*22
		if i == a.sel && a.mode == acBrowse {
			c = pal.White
			if ctx.Tick/20%2 == 0 {
				f.Draw(dst, "►", x+16, iy, 1, pal.Yellow)
			}
		}
		label := acLabels[it]
		if it == acUnlinkItem && school(ctx) {
			label = "Sign out"
		}
		if it == acLink && a.linking {
			label = "Linking…"
		}
		f.DrawShadow(dst, label, x+36, iy, 1, c)
	}
	if a.msg != "" {
		// Up to two lines, the last just above the key help.
		lines := msgLines(f, a.msg)
		for i, l := range lines {
			f.DrawCentered(dst, l, cx, game.ScreenH-44-(len(lines)-1-i)*18, 1, a.msgCol)
		}
	}
	if a.mode == acBrowse {
		// Each dialog draws its own keys.
		f.DrawShadow(dst, "↑/↓ choose   Enter select   Esc back", 8, game.ScreenH-20, 1, pal.Ash)
	}

	switch a.mode {
	case acQR:
		a.drawPair(dst, ctx)
	case acConfirm:
		a.drawConfirm(dst, ctx)
	case acCode:
		dx, dy := dialog(dst, ctx, "LINK THIS GAME", 420, 170)
		f.DrawShadow(dst, "Type the code from the website:", dx, dy, 1, pal.Ice)
		code := showCode(a.code)
		f.DrawCentered(dst, code, cx, dy+30, 3, pal.White)
		f.DrawShadow(dst, "A code works once, for 15 minutes.", dx, dy+76, 1, pal.Steel)
		f.DrawShadow(dst, "Enter link   Esc cancel", dx, dy+96, 1, pal.Ash)
	case acUnlink:
		if school(ctx) {
			drawSignOut(dst, ctx)
			break
		}
		dx, dy := dialog(dst, ctx, "UNLINK?", 460, 170)
		f.DrawShadow(dst, "Your progress stays in this game, and assigned", dx, dy, 1, pal.Ice)
		f.DrawShadow(dst, "lists become your own (except bought ones).", dx, dy+16, 1, pal.Ice)
		f.DrawShadow(dst, "Answers not yet sent won't reach the website.", dx, dy+32, 1, pal.Tan)
		f.DrawShadow(dst, "Y unlink   N cancel", dx, dy+72, 1, pal.Ash)
	}
}

// syncProblem is the line saying why the last sync didn't work, or ""
// when it did. A learner who stopped their own progress being kept gets
// a calm line: the game stays linked and works as before. A suspended account (halpwords-server#130) gets a calm
// line for the child rather than an error: the game stays linked and
// picks up by itself once a grown-up sorts it out.
//
// A suspended, removed or paused account, or another error, says more
// than being stopped does, so it comes first (PLAN §14, 2026-10-12).
func syncProblem(st link.Status) (string, color.RGBA) {
	switch {
	case st.Suspended:
		if st.Stopped {
			return "Can't sync right now. Ask your grown-up.", pal.Ice
		}
		return "Can't sync right now. Ask your grown-up. Your progress is kept.", pal.Ice
	case st.Offline:
		return "Can't reach the website. The game keeps trying.", pal.Tan
	case st.Err != nil:
		return upperFirst(link.Explain(st.Err)) + ".", pal.Tan
	case st.Stopped:
		return "Your progress isn't being kept, so this game doesn't sync. It still works here.", pal.Ice
	}
	return "", color.RGBA{}
}
