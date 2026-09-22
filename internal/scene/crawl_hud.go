package scene

import (
	"fmt"
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/combat"
	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/typing"
)

const exploreHelp = "↑↓ walk  ←→ turn  Q/E strafe  Space use  P potion  M map  Esc menu"

// drawViewOverlay draws text and gauges over the 3D view.
func (c *Crawl) drawViewOverlay(view *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	vw, vh := viewW*gfx.ArtScale, viewH*gfx.ArtScale
	cx := viewX + vw/2

	// Compass.
	gfx.FillRect(view, viewX+vw-30, viewY+4, 26, 22, pal.Fade(pal.Black, 0.6))
	f.DrawCentered(view, c.facing.String(), viewX+vw-17, viewY+7, 1, pal.Yellow)

	for _, fl := range c.floats {
		a := min(1, float64(50-fl.t)/15)
		f.DrawOutline(view, fl.text, cx-f.Width(fl.text, 2)/2, viewY+vh/2-30-fl.t, 2, pal.Fade(fl.col, a), pal.Fade(pal.Black, a))
	}

	switch c.mode {
	case modeBattle:
		b := c.battle
		m := b.m
		gfx.FillRect(view, viewX, viewY, vw, 34, pal.Fade(pal.Black, 0.55))
		f.DrawCentered(view, m.Name(), cx, viewY+2, 1, pal.White)
		bar(view, cx-80, viewY+21, 160, 8, float64(max(0, m.HP))/float64(m.MaxHP), pal.Rose, pal.Plum)
		c.drawHint(view, ctx, c.battleHelp())
	case modePuzzle:
		c.drawHint(view, ctx, c.puzzleHelp())
	case modeMap:
		gfx.FillRect(view, viewX, viewY, vw, vh, pal.Fade(pal.Black, 0.85))
		cell := max(3, min((vw-8)/c.level.W, (vh-8)/c.level.H))
		c.drawAutomap(view, viewX+4, viewY+4, vw-8, vh-8, cell, true)
		c.drawHint(view, ctx, "M or Esc close")
	case modeQuit:
		c.drawDialog(view, ctx, "Leave the dungeon?", "Your progress will be lost.", "Y leave · N stay")
	case modeDead:
		gfx.FillRect(view, viewX, viewY, vw, vh, pal.Fade(pal.Red, 0.35))
		c.drawDialog(view, ctx, "YOU HAVE FALLEN", fmt.Sprintf("Wake at the start of floor %d?", c.run.depth), "Enter try again · Esc give up")
	}

	if c.bannerT > 0 {
		a := min(1, float64(c.bannerT)/30)
		y := viewY + 70
		if c.mode == modeBattle {
			y = viewY + 44
		}
		f.DrawOutline(view, c.banner, cx-f.Width(c.banner, 3)/2, y, 3, pal.Fade(pal.Yellow, a), pal.Fade(pal.Black, a))
		if c.sub != "" {
			f.DrawOutline(view, c.sub, cx-f.Width(c.sub, 2)/2, y+52, 2, pal.Fade(pal.Tan, a), pal.Fade(pal.Black, a))
		}
	}
}

// drawHint draws a line of key help along the bottom of the view.
func (c *Crawl) drawHint(view *ebiten.Image, ctx *game.Context, s string) {
	if s == "" {
		return
	}
	vw, vh := viewW*gfx.ArtScale, viewH*gfx.ArtScale
	gfx.FillRect(view, viewX, viewY+vh-22, vw, 22, pal.Fade(pal.Black, 0.6))
	ctx.Font.DrawCentered(view, s, viewX+vw/2, viewY+vh-19, 1, pal.Tan)
}

func (c *Crawl) drawDialog(view *ebiten.Image, ctx *game.Context, title, text, keys string) {
	f := ctx.Font
	vw, vh := viewW*gfx.ArtScale, viewH*gfx.ArtScale
	w, h := vw-48, 100
	x, y := viewX+24, viewY+vh/2-h/2
	gfx.Window(view, x, y, w, h)
	f.DrawCentered(view, title, x+w/2, y+12, 2, pal.Yellow)
	f.DrawCentered(view, text, x+w/2, y+50, 1, pal.Ice)
	f.DrawCentered(view, keys, x+w/2, y+72, 1, pal.Tan)
}

// drawSide draws the hero's stats and the automap to the right of the view.
func (c *Crawl) drawSide(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	h := &c.run.hero
	x, y, w := sideX, 4, sideW
	gfx.Window(dst, x, y, w, 116)
	x += 12
	w -= 24
	f.DrawShadow(dst, fmt.Sprintf("Level %d", h.Level), x, y+9, 1, pal.Yellow)
	lang := c.run.lang.Name
	f.DrawShadow(dst, lang, x+w-f.Width(lang, 1), y+9, 1, pal.Tan)

	hpCol := pal.Lime
	switch {
	case h.HP*4 <= h.MaxHP:
		hpCol = pal.Rose
	case h.HP*2 <= h.MaxHP:
		hpCol = pal.Yellow
	}
	f.DrawShadow(dst, "HP", x, y+29, 1, pal.Steel)
	bar(dst, x+24, y+32, 110, 10, float64(max(0, h.HP))/float64(h.MaxHP), hpCol, pal.Plum)
	f.DrawShadow(dst, fmt.Sprintf("%d/%d", max(0, h.HP), h.MaxHP), x+142, y+29, 1, pal.Ice)
	f.DrawShadow(dst, "XP", x, y+47, 1, pal.Steel)
	bar(dst, x+24, y+50, 110, 10, float64(h.XP)/float64(h.NextXP()), pal.Sky, pal.Navy)
	f.DrawShadow(dst, fmt.Sprintf("%d/%d", h.XP, h.NextXP()), x+142, y+47, 1, pal.Ice)

	f.DrawShadow(dst, fmt.Sprintf("ATK %d", h.ATK), x, y+67, 1, pal.Ice)
	f.DrawShadow(dst, fmt.Sprintf("Gold %d", h.Gold), x+96, y+67, 1, pal.Yellow)
	f.DrawShadow(dst, fmt.Sprintf("Potions %d", h.Potions), x, y+85, 1, pal.Pink)
	combo, col := "Combo -", pal.Ash
	if h.Streak > 0 {
		combo, col = fmt.Sprintf("Combo ×%.1f", combat.Combo(h.Streak)), pal.Lime
	}
	f.DrawShadow(dst, combo, x+96, y+85, 1, col)

	y = 124
	x, w = sideX, sideW
	gfx.Window(dst, x, y, w, 128)
	if fld := c.typingField(); fld != nil && fld.Greek {
		drawGreekChart(dst, ctx, x+10, y+8)
		return
	}
	title := fmt.Sprintf("Floor %d · %s", c.run.depth, c.theme.Name)
	f.DrawShadow(dst, title, x+10, y+7, 1, pal.Tan)
	c.drawAutomap(dst, x+6, y+26, w-12, 128-32, 8, false)
}

// typingField returns the field the hero is typing into, if any.
func (c *Crawl) typingField() *typing.Field {
	switch {
	case c.mode == modeBattle && (c.battle.phase == phaseAttack || c.battle.phase == phaseDefend):
		return c.battle.field
	case c.mode == modePuzzle && !c.puzzle.showing:
		return c.puzzle.field
	}
	return nil
}

func drawGreekChart(dst *ebiten.Image, ctx *game.Context, x, y int) {
	f := ctx.Font
	f.DrawShadow(dst, "Greek keys (F2 off)", x, y, 1, pal.Tan)
	for i, k := range typing.BetaCodeChart {
		cx, cy := x+(i%6)*36, y+18+(i/6)*16
		f.Draw(dst, string(k.Key), cx, cy, 1, pal.Steel)
		f.Draw(dst, string(k.Greek), cx+12, cy, 1, pal.White)
	}
	f.DrawShadow(dst, ")ἀ (ἁ /ά \\ὰ =ᾶ |ᾳ +ϊ", x, y+84, 1, pal.Ice)
	f.DrawShadow(dst, "after a vowel · Tab accent", x, y+100, 1, pal.Ash)
}

// drawAutomap draws the cells the hero has seen, with cell pixels per cell.
// The small map follows the hero; the full map is centred on the floor.
func (c *Crawl) drawAutomap(dst *ebiten.Image, x, y, w, h, cell int, full bool) {
	l := c.level
	clip := dst.SubImage(image.Rect(x, y, x+w, y+h)).(*ebiten.Image)
	var ox, oy int
	if full {
		ox, oy = x+w/2-l.W*cell/2, y+h/2-l.H*cell/2
	} else {
		ox, oy = x+w/2-c.pos.X*cell-cell/2, y+h/2-c.pos.Y*cell-cell/2
	}
	for cy := 0; cy < l.H; cy++ {
		for cx := 0; cx < l.W; cx++ {
			p := dungeon.Point{X: cx, Y: cy}
			i := l.Index(p)
			if !l.Seen[i] {
				continue
			}
			px, py := ox+cx*cell, oy+cy*cell
			if px+cell < x || py+cell < y || px > x+w || py > y+h {
				continue
			}
			switch l.At(p) {
			case dungeon.Wall:
				gfx.FillRect(clip, px, py, cell, cell, pal.Granite)
				if l.Torches[i] {
					gfx.FillRect(clip, px+cell/2-1, py+cell/2-1, 2, 2, pal.Orange)
				}
			case dungeon.Floor:
				gfx.FillRect(clip, px, py, cell, cell, pal.Indigo)
			case dungeon.Door:
				gfx.FillRect(clip, px, py, cell, cell, pal.Brown)
			case dungeon.Sealed:
				gfx.FillRect(clip, px, py, cell, cell, pal.Purple)
				gfx.FillRect(clip, px+1, py+1, cell-2, cell-2, pal.Pink)
			case dungeon.OpenDoor:
				gfx.FillRect(clip, px, py, cell, cell, pal.Indigo)
				gfx.FillRect(clip, px+1, py+1, cell-2, cell-2, pal.Bronze)
			case dungeon.Stairs:
				gfx.FillRect(clip, px, py, cell, cell, pal.Lime)
			}
			if ch := l.Chests[p]; ch != nil {
				col := pal.Yellow
				if ch.Open {
					col = pal.Bronze
				}
				gfx.FillRect(clip, px+1, py+1, cell-2, cell-2, col)
			}
		}
	}
	for _, m := range l.Monsters {
		if m.HP <= 0 || !l.Seen[l.Index(m.At)] || m.At.Manhattan(c.pos) > 6 {
			continue
		}
		px, py := ox+m.At.X*cell, oy+m.At.Y*cell
		gfx.FillRect(clip, px+1, py+1, cell-2, cell-2, pal.Rose)
	}
	// The hero: a white dot with a yellow nose pointing the way they face.
	px, py := ox+c.pos.X*cell, oy+c.pos.Y*cell
	gfx.FillRect(clip, px, py, cell, cell, pal.White)
	d := c.facing.Delta()
	nx, ny := px+cell/2-1+d.X*(cell/2+1), py+cell/2-1+d.Y*(cell/2+1)
	gfx.FillRect(clip, nx, ny, 2, 2, pal.Yellow)
}

// drawPanel draws the bottom panel: the message log while exploring, or the
// word to type in a battle or puzzle.
func (c *Crawl) drawPanel(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	x, y, w, h := 4, panelY, game.ScreenW-8, game.ScreenH-panelY-4
	gfx.Window(dst, x, y, w, h)
	switch {
	case c.mode == modeBattle:
		c.drawBattlePanel(dst, ctx, x, y, w)
		return
	case c.mode == modePuzzle:
		c.drawPuzzlePanel(dst, ctx, x, y, w)
		return
	}
	log := c.run.log[max(0, len(c.run.log)-4):]
	for i, l := range log {
		col := l.col
		if i < len(log)-1 {
			col = pal.Fade(col, 0.6)
		}
		f.DrawShadow(dst, l.text, x+12, y+9+i*16, 1, col)
	}
	f.DrawShadow(dst, exploreHelp, x+12, y+h-24, 1, pal.Ash)
}
