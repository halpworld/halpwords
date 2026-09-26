package scene

import (
	"fmt"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/pkg/compete"
	"github.com/halpworld/halpwords/pkg/words"
)

// Adventure picks the language for a new dungeon run.
type Adventure struct {
	bg    *ebiten.Image
	setup runSetup
	langs []*words.Language
	count []int // words per language
	sel   int
}

// NewAdventure creates the language picker for a run set up by the mode
// picker.
func NewAdventure(ctx *game.Context, setup runSetup) game.Scene {
	a := &Adventure{bg: backdrop(3, 1.4), setup: setup}
	for _, l := range words.Languages {
		n := 0
		for _, list := range ctx.ListsFor(l.Code) {
			n += len(list.Entries)
		}
		if n > 0 {
			a.langs = append(a.langs, l)
			a.count = append(a.count, n)
		}
	}
	return a
}

// Update implements game.Scene.
func (a *Adventure) Update(ctx *game.Context) error {
	switch {
	case input.Back() && a.setup.quest != nil:
		ctx.Sound.Play(audio.Back)
		ctx.Replace(NewQuests(ctx))
	case input.Back():
		ctx.Sound.Play(audio.Back)
		ctx.Replace(NewNewGame(ctx))
	case len(a.langs) == 0:
	case input.Up():
		ctx.Sound.Play(audio.Blip)
		a.sel = (a.sel + len(a.langs) - 1) % len(a.langs)
	case input.Down():
		ctx.Sound.Play(audio.Blip)
		a.sel = (a.sel + 1) % len(a.langs)
	case input.Confirm() || input.Pressed(ebiten.KeySpace):
		ctx.Sound.Play(audio.Select)
		setup := a.setup
		if setup.mode == compete.Daily {
			setup = dailySetup(ctx, a.langs[a.sel])
		}
		ctx.Replace(NewClassPick(a.langs[a.sel], setup))
	}
	return nil
}

// Draw implements game.Scene.
func (a *Adventure) Draw(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	gfx.DrawArt(dst, a.bg, 0, 0)
	f.DrawCentered(dst, "Choose your language", cx, 40, 3, pal.Yellow)
	f.DrawCentered(dst, "Monsters, doors and chests will ask you for words in it.", cx, 96, 1, pal.Tan)
	f.DrawCentered(dst, setupText(a.setup), cx, 112, 1, pal.Ice)

	if len(a.langs) == 0 {
		f.DrawCentered(dst, "No word lists found. Add some to the words folder.", cx, 170, 1, pal.Rose)
		return
	}
	const w = 400
	h := 28*len(a.langs) + 28
	x, y := cx-w/2, 130
	gfx.Window(dst, x, y, w, h)
	for i, l := range a.langs {
		ly := y + 18 + i*28
		c := pal.Steel
		if i == a.sel {
			c = pal.White
			if ctx.Tick/20%2 == 0 {
				f.Draw(dst, "►", x+18, ly, 2, pal.Yellow)
			}
		}
		f.DrawShadow(dst, l.Name, x+44, ly, 2, c)
		n := fmt.Sprintf("%d words", a.count[i])
		f.DrawShadow(dst, n, x+w-20-f.Width(n, 1), ly+8, 1, pal.Ash)
	}
	f.DrawShadow(dst, "↑/↓ choose   Enter next   Esc back", 8, game.ScreenH-20, 1, pal.Ash)
}

// setupText describes how a new run will be played, such as "Hardcore ·
// seed 7K3QZP".
func setupText(s runSetup) string {
	switch {
	case s.quest != nil:
		return "Quest · " + s.quest.Title
	case s.mode == compete.Daily && s.day != "":
		return "Daily Dungeon · " + s.day
	case s.mode == compete.Daily:
		return "Daily Dungeon · " + time.Now().Format(time.DateOnly)
	case s.seeded:
		return s.mode.String() + " · seed " + compete.SeedCode(s.seed)
	}
	return s.mode.String()
}
