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
const Version = "v0.4 (milestone 3 started: puzzles)"

// Title is the title screen and main menu.
type Title struct {
	bg      *ebiten.Image
	torches []*gfx.Torch
	sel     int
	items   []titleItem
	saved   string // describes the saved adventure, if there is one
}

// titleItem is an entry in the main menu.
type titleItem int

const (
	titleContinue titleItem = iota
	titleNew
	titlePractice
	titleWordLists
	titleQuit
)

var titleLabels = [...]string{"Continue", "New Adventure", "Spelling Practice", "Word Lists", "Quit"}

// NewTitle creates the title screen.
func NewTitle(*game.Context) game.Scene {
	t := &Title{
		bg:      backdrop(1, 1.1),
		torches: []*gfx.Torch{gfx.NewTorch(96, 150, 1), gfx.NewTorch(game.ScreenW-96, 150, 2)},
		items:   []titleItem{titleNew, titlePractice, titleWordLists, titleQuit},
	}
	if s, ok := saveSummary(); ok {
		t.saved = s
		t.items = append([]titleItem{titleContinue}, t.items...)
	}
	return t
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
		switch t.items[t.sel] {
		case titleContinue:
			c, err := loadCrawl(ctx)
			if err != nil {
				ctx.Sound.Play(audio.Wrong)
				ctx.Notify("Can't continue: " + err.Error())
				return nil
			}
			ctx.Sound.Play(audio.Select)
			ctx.Replace(c)
		case titleNew:
			ctx.Sound.Play(audio.Select)
			ctx.Replace(NewAdventure(ctx))
		case titlePractice:
			ctx.Sound.Play(audio.Select)
			ctx.Replace(NewPractice(ctx))
		case titleWordLists:
			ctx.Sound.Play(audio.Select)
			ctx.Replace(NewWordLists(ctx))
		case titleQuit:
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

	// Menu. The saved adventure is described next to Continue.
	mw := 0
	for _, it := range t.items {
		w := f.Width(titleLabels[it], 2)
		if it == titleContinue {
			w += f.Width(t.saved, 1) + 24
		}
		mw = max(mw, w)
	}
	mw += 88
	step := 28
	if len(t.items) > 4 {
		step = 26
	}
	mh := step*len(t.items) + 24
	mx, my := game.ScreenW/2-mw/2, min(205, game.ScreenH-26-mh)
	gfx.Window(dst, mx, my, mw, mh)
	for i, it := range t.items {
		y := my + 14 + i*step
		c := pal.Steel
		if i == t.sel {
			c = pal.White
			if ctx.Tick/20%2 == 0 {
				f.Draw(dst, "►", mx+18, y, 2, pal.Yellow)
			}
		}
		f.DrawShadow(dst, titleLabels[it], mx+44, y, 2, c)
		if it == titleContinue {
			f.DrawShadow(dst, t.saved, mx+mw-20-f.Width(t.saved, 1), y+8, 1, pal.Ash)
		}
	}

	f.DrawShadow(dst, "↑/↓ choose   Enter select", 8, game.ScreenH-20, 1, pal.Ash)
	f.DrawShadow(dst, Version, game.ScreenW-8-f.Width(Version, 1), game.ScreenH-20, 1, pal.Ash)
}
