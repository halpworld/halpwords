package scene

import (
	"fmt"
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/combat"
	"github.com/halpworld/halpwords/internal/compete"
	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/internal/typing"
)

const exploreHelp = "↑↓ walk ←→ turn Q/E strafe Space use P potion I items M map Esc menu"

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
		// The kind's traits follow the name; an extra trait is already in it.
		name, tags := m.Name(), (m.Traits &^ m.Extra).String()
		if tags != "" {
			tags = " · " + tags
		}
		nx := cx - (f.Width(name, 1)+f.Width(tags, 1))/2
		f.Draw(view, name, nx, viewY+2, 1, pal.White)
		f.Draw(view, tags, nx+f.Width(name, 1), viewY+2, 1, pal.Cyan)
		bar(view, cx-80, viewY+21, 160, 8, float64(max(0, m.HP))/float64(m.MaxHP), pal.Rose, pal.Plum)
		if b.hints > 0 && (b.phase == phaseAttack || b.phase == phaseDefend) {
			c.drawLetterHint(view, ctx, hintText(b.word.Answers[0], b.hints))
		}
		c.drawBuffs(view, ctx)
		if b.tauntT > 0 && b.hints == 0 {
			// The monster's taunt, low in the view, clear of the banner.
			a := min(1, float64(b.tauntT)/30)
			text := "“" + b.taunt.Text + "”"
			f.DrawOutline(view, text, cx-f.Width(text, 1)/2, viewY+vh-64, 1, pal.Fade(pal.Pink, a), pal.Fade(pal.Black, a))
			en := "(" + b.taunt.English + ")"
			f.DrawOutline(view, en, cx-f.Width(en, 1)/2, viewY+vh-46, 1, pal.Fade(pal.Ice, a), pal.Fade(pal.Black, a))
		}
		c.drawHint(view, ctx, c.battleHelp())
	case modePuzzle:
		if c.puzzle.fields != nil {
			c.drawCrossword(view, ctx)
		}
		if lp := c.puzzle; lp.hints > 0 && !lp.showing {
			c.drawLetterHint(view, ctx, hintText(puzzleAnswer(lp.p), lp.hints))
		}
		c.drawHint(view, ctx, c.puzzleHelp())
	case modeMap:
		gfx.FillRect(view, viewX, viewY, vw, vh, pal.Fade(pal.Black, 0.85))
		cell := max(3, min((vw-8)/c.level.W, (vh-8)/c.level.H))
		c.drawAutomap(view, viewX+4, viewY+4, vw-8, vh-8, cell, true)
		c.drawHint(view, ctx, "M or Esc close")
	case modePause:
		c.drawPause(view, ctx)
	case modeQuit:
		title, text := "Quit without saving?", "You will go back to your last shrine."
		switch {
		case c.run.hardcore():
			title, text = "Give up this run?", fmt.Sprintf("Your score of %s will be recorded.", groupDigits(c.run.score()))
		case c.lastSave == nil:
			text = "This adventure has not been saved."
		}
		c.drawDialog(view, ctx, title, text, "Y yes · N go back")
	case modeShrine:
		c.drawDialog(view, ctx, "SAVE SHRINE", "Pray here to save your adventure?", "Y pray · N leave")
	case modeCampfire:
		c.drawCampfire(view, ctx)
	case modeDead:
		gfx.FillRect(view, viewX, viewY, vw, vh, pal.Fade(pal.Red, 0.35))
		if c.run.hardcore() {
			c.drawDialog(view, ctx, "YOU HAVE FALLEN", fmt.Sprintf("Floor %d · score %s", c.run.depth, groupDigits(c.run.score())), "Enter see your score")
			break
		}
		c.drawDialog(view, ctx, "YOU HAVE FALLEN", c.wakeText(), "Enter try again · Esc give up")
	}

	if c.bannerT > 0 && c.mode != modePause && c.mode != modeQuit && c.mode != modeCampfire && c.mode != modeShrine {
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

// drawLetterHint shows the letters hints have given, above the key help.
func (c *Crawl) drawLetterHint(view *ebiten.Image, ctx *game.Context, text string) {
	f := ctx.Font
	vw, vh := viewW*gfx.ArtScale, viewH*gfx.ArtScale
	text = "Hint: " + text
	sc := f.FitScale(text, vw-16, 2)
	f.DrawOutline(view, text, viewX+vw/2-f.Width(text, sc)/2, viewY+vh-26-16*sc, sc, pal.Yellow, pal.Black)
}

// drawBuffs marks an Hourglass or Rune of Clarity working in a battle.
func (c *Crawl) drawBuffs(view *ebiten.Image, ctx *game.Context) {
	b := c.battle
	y := viewY + 38
	for _, buff := range []struct {
		on   bool
		text string
	}{{b.slow, "Hourglass"}, {b.clarity, "Clarity"}} {
		if buff.on {
			ctx.Font.DrawOutline(view, buff.text, viewX+6, y, 1, pal.Cyan, pal.Black)
			y += 16
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
	f.DrawShadow(dst, fmt.Sprintf("Lv %d %s", h.Level, h.Class), x, y+7, 1, pal.Yellow)
	lang := c.run.lang.Name
	f.DrawShadow(dst, lang, x+w-f.Width(lang, 1), y+7, 1, pal.Tan)

	maxHP := h.MaxHP()
	hpCol := pal.Lime
	switch {
	case h.HP*4 <= maxHP:
		hpCol = pal.Rose
	case h.HP*2 <= maxHP:
		hpCol = pal.Yellow
	}
	gauge := func(label string, gy, v, most int, fg, bg color.RGBA) {
		f.DrawShadow(dst, label, x, gy, 1, pal.Steel)
		bar(dst, x+24, gy+3, 110, 10, float64(max(0, v))/float64(max(1, most)), fg, bg)
		f.DrawShadow(dst, fmt.Sprintf("%d/%d", max(0, v), most), x+142, gy, 1, pal.Ice)
	}
	gauge("HP", y+24, h.HP, maxHP, hpCol, pal.Plum)
	gauge("MP", y+40, h.MP, h.MaxMP(), pal.Cyan, pal.Indigo)
	gauge("XP", y+56, h.XP, h.NextXP(), pal.Sky, pal.Navy)

	f.DrawShadow(dst, fmt.Sprintf("ATK %d  DEF %d", h.ATK(), h.DEF()), x, y+74, 1, pal.Ice)
	gold := fmt.Sprintf("Gold %d", h.Gold)
	f.DrawShadow(dst, gold, x+w-f.Width(gold, 1), y+74, 1, pal.Yellow)
	f.DrawShadow(dst, fmt.Sprintf("Potions %d", h.Items[rpg.Potion]), x, y+92, 1, pal.Pink)
	combo, col := "Combo -", pal.Ash
	if h.Streak > 0 {
		combo, col = fmt.Sprintf("Combo ×%.1f", combat.Combo(h.Streak)), pal.Lime
	}
	f.DrawShadow(dst, combo, x+w-f.Width(combo, 1), y+92, 1, col)

	y = 124
	x, w = sideX, sideW
	gfx.Window(dst, x, y, w, 128)
	if fld := c.typingField(); fld != nil && fld.Greek {
		drawGreekChart(dst, ctx, x+10, y+8)
		return
	}
	title := fmt.Sprintf("Floor %d · %s", c.run.depth, c.floorName())
	if c.run.hardcore() {
		title = fmt.Sprintf("Floor %d · Score %s", c.run.depth, groupDigits(c.run.score()))
		if best := ctx.Profile.Fame.Best(compete.TableKey(c.run.mode, c.run.lang.Code)); best > 0 && c.run.score() > best {
			f.DrawShadow(dst, "★ BEST", x+w-10-f.Width("★ BEST", 1), y+7, 1, pal.Yellow)
		}
	}
	f.DrawShadow(dst, fit(f, title, w-20, 1), x+10, y+7, 1, pal.Tan)
	c.drawAutomap(dst, x+6, y+26, w-12, 128-32, 8, false)
}

// typingField returns the field the hero is typing into, if any.
func (c *Crawl) typingField() *typing.Field {
	switch {
	case c.mode == modeBattle && (c.battle.phase == phaseAttack || c.battle.phase == phaseDefend):
		return c.battle.field
	case c.mode == modePuzzle && !c.puzzle.showing && c.puzzle.field != nil:
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
			if ft := l.Features[p]; ft != nil {
				col := [...]color.RGBA{pal.Cyan, pal.Orange, pal.Pink}[ft.Kind]
				if ft.Kind == dungeon.Campfire && ft.Used {
					col = pal.Mahogany
				}
				gfx.FillRect(clip, px+1, py+1, cell-2, cell-2, col)
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
		if m.HP <= 0 || !l.Seen[l.Index(m.At)] || (m.At.Manhattan(c.pos) > 6 && !m.Kind.Boss()) {
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
	if c.mode == modeExplore {
		f.DrawShadow(dst, exploreHelp, x+12, y+h-24, 1, pal.Ash)
	}
}
