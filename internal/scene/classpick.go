package scene

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/pkg/compete"
	"github.com/halpworld/halpwords/pkg/proc"
	"github.com/halpworld/halpwords/pkg/words"
)

// ClassPick chooses the hero's class for a new adventure.
type ClassPick struct {
	bg    *ebiten.Image
	lang  *words.Language
	setup runSetup
	heros []*ebiten.Image
	sel   int
	// ask is set while the player is asked whether a new run may replace
	// the saved adventure; yes is the answer chosen, and starts as No.
	ask, yes bool
}

// confirming reports whether the player is being asked to replace a save.
func (p *ClassPick) confirming() bool { return p.ask }

// replaceText says what starting a new run does to the saved adventure,
// such as "This replaces your saved adventure: Latin · Knight · Floor 4".
func replaceText() string {
	sum, _ := saveSummary()
	if sum == "" {
		return "This replaces your saved adventure."
	}
	return "This replaces your saved adventure: " + sum
}

// replaceNote is said as well when the saved run can't be got back.
const replaceNote = "This run can't be undone."

// updateAsk answers the question of replacing the saved adventure. Nothing
// is replaced unless the player chooses Yes.
func (p *ClassPick) updateAsk(ctx *game.Context) {
	switch {
	case input.Pressed(ebiten.KeyY):
		p.yes = true
		fallthrough
	case input.Confirm() || input.Pressed(ebiten.KeySpace):
		if p.yes {
			p.begin(ctx)
			return
		}
		ctx.Sound.Play(audio.Back)
		p.ask = false
	case input.Back() || input.Pressed(ebiten.KeyN):
		ctx.Sound.Play(audio.Back)
		p.ask, p.yes = false, false
	case input.Up() || input.Down() || input.Repeat(ebiten.KeyArrowLeft) || input.Repeat(ebiten.KeyArrowRight) ||
		input.Repeat(ebiten.KeyA) || input.Repeat(ebiten.KeyD):
		ctx.Sound.Play(audio.Blip)
		p.yes = !p.yes
	}
}

// begin starts the run with the hero chosen.
func (p *ClassPick) begin(ctx *game.Context) {
	r := newRun(ctx, p.lang, rpg.Classes[p.sel], p.setup)
	if r.quest != nil {
		ctx.Replace(questIntro(r, p.setup))
		return
	}
	ctx.Replace(newCrawl(r))
}

// NewClassPick creates the class picker for an adventure in lang.
func NewClassPick(lang *words.Language, setup runSetup) game.Scene {
	p := &ClassPick{bg: backdrop(4, 1.4), lang: lang, setup: setup}
	for i := range rpg.Classes {
		p.heros = append(p.heros, gfx.Upload(proc.HeroSprite(i).RGBA()))
	}
	return p
}

// Update implements game.Scene.
func (p *ClassPick) Update(ctx *game.Context) error {
	if p.ask {
		p.updateAsk(ctx)
		return nil
	}
	n := len(rpg.Classes)
	switch {
	case input.Back() && p.setup.picked:
		ctx.Sound.Play(audio.Back)
		ctx.Replace(adventureLists(ctx, p.lang, p.setup))
	case input.Back() && p.setup.quest != nil && p.setup.quest.Language != "":
		ctx.Sound.Play(audio.Back)
		ctx.Replace(NewQuests(ctx))
	case input.Back():
		ctx.Sound.Play(audio.Back)
		setup := p.setup
		if setup.assign != nil {
			ctx.Replace(NewAssignments(ctx))
			return nil
		}
		if setup.mode == compete.Daily {
			setup = runSetup{mode: compete.Daily} // the language decides the dungeon
		}
		ctx.Replace(NewAdventure(ctx, setup))
	case input.Repeat(ebiten.KeyArrowLeft) || input.Repeat(ebiten.KeyA) || input.Up():
		ctx.Sound.Play(audio.Blip)
		p.sel = (p.sel + n - 1) % n
	case input.Repeat(ebiten.KeyArrowRight) || input.Repeat(ebiten.KeyD) || input.Down():
		ctx.Sound.Play(audio.Blip)
		p.sel = (p.sel + 1) % n
	case input.Confirm() || input.Pressed(ebiten.KeySpace):
		if _, _, ok := savedHero(); ok {
			// There is one save slot, and the new run takes it.
			ctx.Sound.Play(audio.Blip)
			p.ask, p.yes = true, false
			return nil
		}
		ctx.Sound.Play(audio.Select)
		p.begin(ctx)
	}
	return nil
}

// Draw implements game.Scene.
func (p *ClassPick) Draw(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	gfx.DrawArt(dst, p.bg, 0, 0)
	f.DrawCentered(dst, "Choose your hero", cx, 24, 3, pal.Yellow)
	f.DrawCentered(dst, setupText(p.setup)+" in "+p.lang.Name, cx, 76, 1, pal.Tan)

	const w, h, gap = 196, 236, 10
	x0 := cx - (3*w+2*gap)/2
	for i, c := range rpg.Classes {
		info := c.Info()
		x, y := x0+i*(w+gap), 96
		gfx.Window(dst, x, y, w, h)
		if i == p.sel {
			gfx.FillRect(dst, x+4, y+4, w-8, h-8, pal.Fade(pal.Indigo, 0.8))
		}
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(5, 5)
		op.GeoM.Translate(float64(x+w/2-40), float64(y+10))
		if i != p.sel {
			op.ColorScale.Scale(0.6, 0.6, 0.6, 1)
		}
		dst.DrawImage(p.heros[i], op)
		col := pal.Steel
		if i == p.sel {
			col = pal.White
		}
		f.DrawCentered(dst, info.Name, x+w/2, y+92, 2, col)
		s := info.Start
		for k, line := range []string{
			fmt.Sprintf("HP %d   MP %d", s.MaxHP, s.MaxMP),
			fmt.Sprintf("ATK %d   DEF %d", s.ATK+rpg.StarterWeapon(c).Bonus().ATK, s.DEF),
			fmt.Sprintf("Focus %d   Luck %d", s.Focus, s.Luck),
		} {
			f.DrawCentered(dst, line, x+w/2, y+130+k*16, 1, pal.Ice)
		}
		for k, line := range info.Blurb {
			f.DrawCentered(dst, line, x+w/2, y+184+k*16, 1, pal.Tan)
		}
	}
	if p.ask {
		p.drawAsk(dst, ctx)
		return
	}
	f.DrawShadow(dst, "←/→ choose   Enter begin   Esc back", 8, game.ScreenH-20, 1, pal.Ash)
}

// drawAsk draws the question of replacing the saved adventure over the
// class cards.
func (p *ClassPick) drawAsk(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	gfx.FillRect(dst, 0, 0, game.ScreenW, game.ScreenH, pal.Fade(pal.Black, 0.7))
	const w, h = 560, 140
	x, y := game.ScreenW/2-w/2, game.ScreenH/2-h/2
	gfx.Window(dst, x, y, w, h)
	f.DrawCentered(dst, "Replace your saved adventure?", x+w/2, y+14, 2, pal.Yellow)
	f.DrawCentered(dst, fit(f, replaceText(), w-24, 1), x+w/2, y+46, 1, pal.Ice)
	if savedScored() {
		f.DrawCentered(dst, replaceNote, x+w/2, y+64, 1, pal.Rose)
	}
	for i, label := range []string{"No, go back", "Yes, replace it"} {
		col := pal.Steel
		if (i == 1) == p.yes {
			col = pal.White
			if ctx.Tick/20%2 == 0 {
				f.Draw(dst, "►", x+60+i*190, y+92, 1, pal.Yellow)
			}
		}
		f.DrawShadow(dst, label, x+80+i*190, y+92, 1, col)
	}
	f.DrawShadow(dst, "←/→ choose   Enter select   Y yes   N no   Esc back", 8, game.ScreenH-20, 1, pal.Ash)
}
