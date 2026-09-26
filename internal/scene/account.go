package scene

import (
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
	acBrowse acMode = iota
	acCode          // typing a pairing code
	acUnlink        // confirming an unlink
)

// acItem is an entry in the Account screen's menu.
type acItem int

const (
	acLink acItem = iota
	acSync
	acUnlinkItem
	acBack
	acSchool
)

var acLabels = [...]string{"Link to a grown-up's account", "Sync now", "Unlink", "Back", "Sign in at school"}

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
	msg        string
	msgCol     color.RGBA
}

// NewAccount creates the Account screen.
func NewAccount(ctx *game.Context) game.Scene {
	return &Account{bg: backdrop(11, 1.2)}
}

func (a *Account) items(ctx *game.Context) []acItem {
	if ctx.Link.Linked() {
		return []acItem{acSync, acUnlinkItem, acBack}
	}
	return []acItem{acLink, acSchool, acBack}
}

func (a *Account) say(msg string, c color.RGBA) { a.msg, a.msgCol = msg, c }

// Update implements game.Scene.
func (a *Account) Update(ctx *game.Context) error {
	if a.signingOut != nil {
		if a.signingOut() {
			a.signingOut = nil
			ctx.Replace(NewLearners(ctx))
		}
		return nil
	}
	st := ctx.Link.Status()
	if a.linking && !st.Busy {
		a.linking = false
		switch {
		case st.Linked:
			ctx.Sound.Play(audio.Perfect)
			a.say("Linked! Your progress now goes to your grown-up's account.", pal.Lime)
			a.sel = 0
		default:
			ctx.Sound.Play(audio.Wrong)
			a.say(upperFirst(link.Explain(st.Err))+".", pal.Rose)
			a.mode = acCode
		}
	}
	switch a.mode {
	case acCode:
		a.updateCode(ctx)
		return nil
	case acUnlink:
		switch {
		case input.Pressed(ebiten.KeyY) && school(ctx):
			ctx.Sound.Play(audio.Select)
			a.signingOut = ctx.SignOut()
			a.mode = acBrowse
			a.say("Signing out…", pal.Ice)
		case input.Pressed(ebiten.KeyY):
			ctx.Link.Unlink()
			ctx.Sound.Play(audio.Select)
			a.say("Unlinked. Your progress is still here.", pal.Lime)
			a.mode, a.sel = acBrowse, 0
		case input.Pressed(ebiten.KeyN) || input.Back():
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
			a.code = a.code[:0]
			a.mode = acCode
			a.say("", pal.Steel)
		case acSync:
			ctx.Sound.Play(audio.Select)
			ctx.Link.SyncNow()
			a.say("Syncing…", pal.Ice)
		case acUnlinkItem:
			ctx.Sound.Play(audio.Select)
			a.mode = acUnlink
		case acSchool:
			ctx.Sound.Play(audio.Select)
			ctx.Replace(NewSignIn(ctx, ""))
		case acBack:
			a.leave(ctx)
		}
	}
	return nil
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
		// Wrap a long line at a space.
		for f.Width(s, 1) > w-32 {
			cut := strings.LastIndex(s[:min(len(s), (w-32)/8)], " ")
			if cut <= 0 {
				break
			}
			f.DrawShadow(dst, s[:cut], tx, ty, 1, c)
			ty += 18
			s = s[cut+1:]
		}
		f.DrawShadow(dst, fit(f, s, w-32, 1), tx, ty, 1, c)
		ty += 18
	}
	if st.Linked {
		name := st.Learner
		if name == "" {
			name = "a learner"
		}
		line("Linked to "+name+"'s account.", pal.Lime)
		line(link.SeenByText(st.SeenBy), pal.Ice)
		line("Last sync: "+ago(time.Now(), st.LastSync), pal.Steel)
		switch {
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
		switch {
		case st.Offline:
			line("Can't reach the website. The game keeps trying.", pal.Tan)
		case st.Err != nil:
			line(upperFirst(link.Explain(st.Err))+".", pal.Tan)
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
		f.DrawCentered(dst, a.msg, cx, game.ScreenH-44, 1, a.msgCol)
	}
	f.DrawShadow(dst, "↑/↓ choose   Enter select   Esc back", 8, game.ScreenH-20, 1, pal.Ash)

	switch a.mode {
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
