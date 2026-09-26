package scene

import (
	"fmt"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/move"
	"github.com/halpworld/halpwords/internal/pal"
)

// NewStart is the first scene. In a web browser that has just come from
// the game's old address, it brings the player's progress along first
// (see package move); back from signing in with a school account on the
// website, it is the sign-in screen waiting for the server; otherwise it
// is the title screen.
func NewStart(ctx *game.Context) game.Scene {
	frag := move.Fragment()
	if strings.HasPrefix(frag, move.Key) {
		move.ClearFragment() // so a reload doesn't import again
	} else if s := resumeSSO(ctx); s != nil {
		// Back from signing in with a school account on the website.
		return s
	}
	return startWith(ctx, move.Saves, frag, move.Go)
}

// startWith is NewStart with the store, the fragment and the way to leave
// the page passed in, for tests.
func startWith(ctx *game.Context, st move.Store, frag string, leave func(string) error) game.Scene {
	in, next, err := move.Receive(st, frag)
	switch {
	case err != nil:
		ctx.Notify("Couldn't bring your progress: " + err.Error())
	case next > 0:
		if err := leave(move.SendURL(next)); err != nil {
			ctx.Notify("Couldn't fetch the rest of your progress")
			break
		}
		return &moving{part: next}
	case in != nil:
		here, err := move.Conflicts(st)
		if err != nil {
			ctx.Notify("Couldn't bring your progress: " + err.Error())
			break
		}
		if len(here) > 0 {
			return &moveAsk{st: st, in: in}
		}
		applyMove(ctx, st, in)
	}
	return NewTitle(ctx)
}

// applyMove puts the incoming progress in place and loads it.
func applyMove(ctx *game.Context, st move.Store, in *move.Incoming) {
	if err := in.Apply(st); err != nil {
		ctx.Notify("Couldn't bring your progress: " + err.Error())
		return
	}
	if err := ctx.ReloadSaves(); err != nil {
		ctx.Notify("Couldn't load your progress: " + err.Error())
		return
	}
	ctx.Notify("Your progress has moved in!")
}

// moving shows while the page fetches the next part of a big save from the
// old address.
type moving struct {
	bg   *ebiten.Image // drawn on first use
	part int
}

func (m *moving) Update(ctx *game.Context) error { return nil }

func (m *moving) Draw(dst *ebiten.Image, ctx *game.Context) {
	if m.bg == nil {
		m.bg = backdrop(6, 1.3)
	}
	gfx.DrawArt(dst, m.bg, 0, 0)
	ctx.Font.DrawCentered(dst, "Bringing your progress…", game.ScreenW/2, 150, 2, pal.Yellow)
	ctx.Font.DrawCentered(dst, fmt.Sprintf("Fetching part %d", m.part), game.ScreenW/2, 184, 1, pal.Tan)
}

// moveAsk asks whether progress from the old address may replace the
// progress already in this browser.
type moveAsk struct {
	bg  *ebiten.Image
	st  move.Store
	in  *move.Incoming
	sel int // 0: bring it, 1: keep what is here
}

var moveChoices = [...]string{"Use the progress I brought", "Keep the progress here"}

func (m *moveAsk) Update(ctx *game.Context) error {
	switch {
	case input.Up() || input.Down():
		ctx.Sound.Play(audio.Blip)
		m.sel = 1 - m.sel
	case input.Confirm() || input.Pressed(ebiten.KeySpace):
		ctx.Sound.Play(audio.Select)
		if m.sel == 0 {
			applyMove(ctx, m.st, m.in)
		} else if err := m.in.Discard(m.st); err != nil {
			ctx.Notify("Couldn't forget the progress you brought: " + err.Error())
		}
		ctx.Replace(NewTitle(ctx))
	}
	return nil
}

func (m *moveAsk) Draw(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	if m.bg == nil {
		m.bg = backdrop(6, 1.3)
	}
	gfx.DrawArt(dst, m.bg, 0, 0)
	f.DrawCentered(dst, "Halpwords has moved!", cx, 40, 3, pal.Yellow)
	f.DrawCentered(dst, "You brought your progress from the old address.", cx, 100, 1, pal.Tan)
	f.DrawCentered(dst, "This browser already has some Halpwords progress.", cx, 116, 1, pal.Tan)
	f.DrawCentered(dst, "Only one can stay. Which one?", cx, 132, 1, pal.Tan)
	const w, h = 500, 76
	x, y := cx-w/2, 164
	gfx.Window(dst, x, y, w, h)
	for i, s := range moveChoices {
		c := pal.Steel
		ry := y + 14 + i*28
		if i == m.sel {
			c = pal.White
			if ctx.Tick/20%2 == 0 {
				f.Draw(dst, "►", x+14, ry, 2, pal.Yellow)
			}
		}
		f.DrawShadow(dst, s, x+40, ry, 2, c)
	}
	f.DrawCentered(dst, "Your AI helper keys don't move: set them up again here.", cx, 262, 1, pal.Ash)
	f.DrawShadow(dst, "Arrows choose   Enter select", 8, game.ScreenH-20, 1, pal.Ash)
}
