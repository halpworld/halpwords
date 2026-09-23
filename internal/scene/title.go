package scene

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
)

// Version is shown on the title screen.
const Version = "v0.3 (milestone 2: words and combat)"

// Title is the title screen and main menu.
type Title struct {
	bg      *ebiten.Image
	torches []*gfx.Torch
	sel     int
	items   []string
}

// NewTitle creates the title screen.
func NewTitle(*game.Context) game.Scene {
	return &Title{
		bg:      backdrop(1, 1.1),
		torches: []*gfx.Torch{gfx.NewTorch(96, 150, 1), gfx.NewTorch(game.ScreenW-96, 150, 2)},
		items:   []string{"New Adventure", "Spelling Practice", "Quit"},
	}
}

// Update implements game.Scene.
func (t *Title) Update(ctx *game.Context) error {
	for _, tr := range t.torches {
		tr.Update()
	}
	switch {
	case input.Up():
		ctx.Sound.Play(audio.Blip)
		t.sel = (t.sel + len(t.items) - 1) % len(t.items)
	case input.Down():
		ctx.Sound.Play(audio.Blip)
		t.sel = (t.sel + 1) % len(t.items)
	case input.Confirm() || input.Pressed(ebiten.KeySpace):
		ctx.Sound.Play(audio.Select)
		switch t.sel {
		case 0:
			ctx.Replace(NewAdventure(ctx))
		case 1:
			ctx.Replace(NewPractice(ctx))
		case 2:
			return ebiten.Termination
		}
	}
	return nil
}

// Draw implements game.Scene.
func (t *Title) Draw(dst *ebiten.Image, ctx *game.Context) {
	gfx.DrawArt(dst, t.bg, 0, 0)
	for _, tr := range t.torches {
		tr.Draw(dst)
	}

	// Logo: each letter bobs on its own phase.
	const logo = "HALPWORDS"
	const scale = 5
	f := ctx.Font
	x := game.ScreenW/2 - f.Width(logo, scale)/2
	for i, r := range logo {
		s := string(r)
		dy := int(math.Round(math.Sin(float64(ctx.Tick)*0.05+float64(i)*0.6) * 4))
		f.DrawOutline(dst, s, x+scale, 56+dy+scale, scale, pal.Mahogany, pal.Mahogany) // drop shadow
		f.DrawOutline(dst, s, x, 56+dy, scale, pal.Yellow, pal.Black)
		x += f.Width(s, scale)
	}
	f.DrawCentered(dst, "~ A Dungeon of Words ~", game.ScreenW/2, 150, 2, pal.Tan)

	// Menu.
	mw := 0
	for _, it := range t.items {
		mw = max(mw, f.Width(it, 2))
	}
	mw += 88
	mh := 28*len(t.items) + 28
	mx, my := game.ScreenW/2-mw/2, 205
	gfx.Window(dst, mx, my, mw, mh)
	for i, it := range t.items {
		y := my + 18 + i*28
		c := pal.Steel
		if i == t.sel {
			c = pal.White
			if ctx.Tick/20%2 == 0 {
				f.Draw(dst, "►", mx+18, y, 2, pal.Yellow)
			}
		}
		f.DrawShadow(dst, it, mx+44, y, 2, c)
	}

	f.DrawShadow(dst, "↑/↓ choose   Enter select", 8, game.ScreenH-20, 1, pal.Ash)
	f.DrawShadow(dst, Version, game.ScreenW-8-f.Width(Version, 1), game.ScreenH-20, 1, pal.Ash)
}
