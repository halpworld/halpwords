package scene

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/proc"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/internal/words"
)

// ClassPick chooses the hero's class for a new adventure.
type ClassPick struct {
	bg    *ebiten.Image
	lang  *words.Language
	heros []*ebiten.Image
	sel   int
}

// NewClassPick creates the class picker for an adventure in lang.
func NewClassPick(lang *words.Language) game.Scene {
	p := &ClassPick{bg: backdrop(4, 1.4), lang: lang}
	for i := range rpg.Classes {
		p.heros = append(p.heros, gfx.Upload(proc.HeroSprite(i).RGBA()))
	}
	return p
}

// Update implements game.Scene.
func (p *ClassPick) Update(ctx *game.Context) error {
	n := len(rpg.Classes)
	switch {
	case input.Back():
		ctx.Sound.Play(audio.Back)
		ctx.Replace(NewAdventure(ctx))
	case input.Repeat(ebiten.KeyArrowLeft) || input.Repeat(ebiten.KeyA) || input.Up():
		ctx.Sound.Play(audio.Blip)
		p.sel = (p.sel + n - 1) % n
	case input.Repeat(ebiten.KeyArrowRight) || input.Repeat(ebiten.KeyD) || input.Down():
		ctx.Sound.Play(audio.Blip)
		p.sel = (p.sel + 1) % n
	case input.Confirm() || input.Pressed(ebiten.KeySpace):
		ctx.Sound.Play(audio.Select)
		ctx.Replace(newCrawl(newRun(ctx, p.lang, rpg.Classes[p.sel])))
	}
	return nil
}

// Draw implements game.Scene.
func (p *ClassPick) Draw(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	gfx.DrawArt(dst, p.bg, 0, 0)
	f.DrawCentered(dst, "Choose your hero", cx, 24, 3, pal.Yellow)
	f.DrawCentered(dst, "Adventure in "+p.lang.Name, cx, 76, 1, pal.Tan)

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
	f.DrawShadow(dst, "←/→ choose   Enter begin   Esc back", 8, game.ScreenH-20, 1, pal.Ash)
}
