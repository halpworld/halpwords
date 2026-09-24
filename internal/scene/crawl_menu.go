package scene

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/proc"
	"github.com/halpworld/halpwords/internal/rpg"
)

// menuRow is one line of the items or shop screen.
type menuRow struct {
	label, right string
	about        string
	head         bool // a heading; it can't be chosen
	// act runs when the row is chosen with Enter. It is nil when the row
	// can't be used now.
	act func(ctx *game.Context)
	bag int // the bag item the row shows, or -1
}

// menu is the items screen or the merchant's shop. It is drawn over the
// whole screen, and the game waits while it is open.
type menu struct {
	shop   bool
	sel    int
	top    int // the first row shown, when the list scrolls
	rows   []menuRow
	note   string // the last thing that happened, shown at the bottom
	used   bool   // an item was used in a battle, which takes the turn
	portra *ebiten.Image
}

// openItems shows the hero's items and gear. The game waits, and a battle
// clock stops, until it is closed.
func (c *Crawl) openItems(ctx *game.Context) {
	c.openMenu(ctx, &menu{})
}

// openShop shows the merchant's wares.
func (c *Crawl) openShop(ctx *game.Context) {
	c.play(audio.Select)
	c.run.say("\"Welcome, traveller! Take a look.\"", pal.Tan)
	c.openMenu(ctx, &menu{shop: true})
}

func (c *Crawl) openMenu(ctx *game.Context, m *menu) {
	if c.mode != modePause {
		c.resume, c.pausedAt = c.mode, ctx.Tick
	}
	c.menu = m
	c.mode = modeItems
	if m.shop {
		c.mode = modeShop
	}
	c.buildMenu()
	c.moveMenu(0)
}

// closeMenu goes back to what the hero was doing. Using an item in a battle
// takes the hero's turn.
func (c *Crawl) closeMenu(ctx *game.Context) {
	used := c.menu.used
	c.menu = nil
	c.unpause(ctx)
	if used && c.mode == modeBattle {
		c.beginDefend(ctx)
	}
}

func (c *Crawl) buildMenu() {
	if c.menu.shop {
		c.menu.rows = c.shopRows()
	} else {
		c.menu.rows = c.itemRows()
	}
	c.menu.sel = min(c.menu.sel, len(c.menu.rows)-1)
}

// moveMenu moves the choice by step rows, skipping headings. A step of 0
// moves to the nearest row that is not a heading.
func (c *Crawl) moveMenu(step int) {
	m := c.menu
	n := len(m.rows)
	dir := step
	if dir == 0 {
		dir = 1
	} else {
		m.sel = (m.sel + step + n) % n
	}
	for k := 0; k < n && m.rows[m.sel].head; k++ {
		m.sel = (m.sel + dir + n) % n
	}
}

func (c *Crawl) updateMenu(ctx *game.Context) {
	m := c.menu
	switch {
	case input.Back() || (!m.shop && input.Pressed(ebiten.KeyI)):
		c.play(audio.Back)
		c.closeMenu(ctx)
	case input.Up():
		c.play(audio.Blip)
		c.moveMenu(-1)
	case input.Down():
		c.play(audio.Blip)
		c.moveMenu(1)
	case input.Confirm() || input.Pressed(ebiten.KeySpace):
		row := m.rows[m.sel]
		if row.act == nil {
			c.play(audio.Bump)
			return
		}
		row.act(ctx)
		if c.menu == nil {
			return // the action closed the menu
		}
		c.buildMenu()
		c.moveMenu(0)
		if m.used {
			c.closeMenu(ctx)
		}
	case !m.shop && input.Pressed(ebiten.KeyD, ebiten.KeyDelete):
		if i := m.rows[m.sel].bag; i >= 0 {
			name := c.run.hero.Bag[i].Name()
			c.run.hero.Drop(i)
			c.play(audio.Erase)
			m.note = "You leave the " + name + " behind."
			c.buildMenu()
			c.moveMenu(0)
		}
	}
}

// inBattle reports whether the menu was opened during a battle.
func (c *Crawl) inBattle() bool { return c.resume == modeBattle }

// itemRows lists the hero's items, the gear they wear and their bag.
func (c *Crawl) itemRows() []menuRow {
	h := &c.run.hero
	rows := []menuRow{{label: "Items", head: true, bag: -1}}
	for it := rpg.Item(0); it < rpg.NumItems; it++ {
		it := it
		row := menuRow{label: it.Info().Name, right: fmt.Sprintf("×%d", h.Items[it]), about: it.Info().About, bag: -1}
		if h.Items[it] > 0 && it != rpg.HintScroll {
			row.act = func(*game.Context) { c.useItem(it) }
		}
		if it == rpg.HintScroll {
			row.about += " Press F4 in a battle or puzzle."
		}
		rows = append(rows, row)
	}
	rows = append(rows, menuRow{label: "Equipped", head: true, bag: -1})
	for s := rpg.Slot(0); s < rpg.NumSlots; s++ {
		row := menuRow{label: s.String() + ": none", bag: -1}
		if g := h.Gear[s]; g != nil {
			row.label, row.about = s.String()+": "+g.Name(), g.About()
		}
		rows = append(rows, row)
	}
	if len(h.Bag) > 0 {
		rows = append(rows, menuRow{label: fmt.Sprintf("Bag (%d/%d)", len(h.Bag), rpg.BagSize), head: true, bag: -1})
	}
	for i, g := range h.Bag {
		i := i
		row := menuRow{label: g.Name(), right: g.Slot.String(), about: c.compare(g), bag: i}
		if !c.inBattle() {
			row.act = func(*game.Context) {
				name := h.Bag[i].Name()
				h.Equip(i)
				c.play(audio.Select)
				c.menu.note = "You put on the " + name + "."
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// compare describes gear next to what the hero wears in its slot.
func (c *Crawl) compare(g rpg.Gear) string {
	s := g.About()
	if old := c.run.hero.Gear[g.Slot]; old != nil {
		s += "   (yours: " + old.About() + ")"
	}
	return s
}

// useItem uses one of the hero's items.
func (c *Crawl) useItem(it rpg.Item) {
	h, m := &c.run.hero, c.menu
	switch it {
	case rpg.Potion:
		if h.HP >= h.MaxHP() {
			m.note = "You are already at full health."
			return
		}
		n := h.Heal(h.PotionHeal())
		c.play(audio.Potion)
		m.note = fmt.Sprintf("You drink a potion and recover %d HP.", n)
		c.float(fmt.Sprintf("+%d", n), pal.Lime)
	case rpg.Ether:
		if h.MP >= h.MaxMP() {
			m.note = "Your MP is already full."
			return
		}
		n := h.Restore(h.EtherRestore())
		c.play(audio.Potion)
		m.note = fmt.Sprintf("You drink an ether and recover %d MP.", n)
	case rpg.Hourglass:
		switch {
		case c.battle != nil && c.battle.slow, c.battle == nil && h.Hourglass:
			m.note = "Time is already slowed."
			return
		case c.battle != nil:
			c.battle.slow = true
			m.note = "Time slows around you."
		default:
			h.Hourglass = true
			m.note = "Time will slow in your next battle."
		}
		c.play(audio.Unseal)
	case rpg.Clarity:
		switch {
		case c.battle != nil && c.battle.clarity, c.battle == nil && h.Clarity:
			m.note = "The rune already glows."
			return
		case c.battle != nil:
			c.battle.clarity = true
			m.note = "The rune glows: typos will show as you type."
		default:
			h.Clarity = true
			m.note = "The rune will glow in your next battle."
		}
		c.play(audio.Unseal)
	default:
		return
	}
	h.Items[it]--
	c.run.say(m.note, pal.Lime)
	m.used = c.inBattle()
}

// shopRows lists what the merchant sells, and what they will buy.
func (c *Crawl) shopRows() []menuRow {
	h, ft := &c.run.hero, c.feature
	rows := []menuRow{{label: "For sale", head: true, bag: -1}}
	for it := rpg.Item(0); it < rpg.NumItems; it++ {
		it := it
		info := it.Info()
		row := menuRow{label: info.Name, right: fmt.Sprintf("%d g", info.Price), about: fmt.Sprintf("%s You have %d.", info.About, h.Items[it]), bag: -1}
		if h.Gold >= info.Price {
			row.act = func(*game.Context) {
				h.Gold -= info.Price
				h.Items[it]++
				c.play(audio.Coin)
				c.menu.note = "You buy a " + info.Name + "."
			}
		}
		rows = append(rows, row)
	}
	for i, g := range ft.Stock {
		i, g := i, g
		row := menuRow{label: g.Name(), right: fmt.Sprintf("%d g", g.Price()), about: g.Slot.String() + ": " + c.compare(g), bag: -1}
		if h.Gold >= g.Price() {
			row.act = func(*game.Context) {
				if !h.Take(g) {
					c.play(audio.Wrong)
					c.menu.note = "Your bag is full. Sell something first."
					return
				}
				h.Gold -= g.Price()
				ft.Stock = append(ft.Stock[:i], ft.Stock[i+1:]...)
				c.play(audio.Coin)
				c.menu.note = "You buy the " + g.Name() + "."
				if h.Gear[g.Slot] != nil && *h.Gear[g.Slot] == g {
					c.menu.note += " You put it on."
				} else {
					c.menu.note += " It goes in your bag."
				}
			}
		}
		rows = append(rows, row)
	}
	if len(h.Bag) > 0 {
		rows = append(rows, menuRow{label: "Sell from your bag", head: true, bag: -1})
	}
	for i, g := range h.Bag {
		i, g := i, g
		rows = append(rows, menuRow{label: g.Name(), right: fmt.Sprintf("+%d g", g.SellPrice()), about: g.Slot.String() + ": " + g.About(), bag: -1,
			act: func(*game.Context) {
				h.Gold += g.SellPrice()
				h.Drop(i)
				c.play(audio.Coin)
				c.menu.note = fmt.Sprintf("You sell the %s for %d gold.", g.Name(), g.SellPrice())
			}})
	}
	return rows
}

// Layout of the menu screen.
const (
	menuX, menuY, menuW, menuH = 12, 12, game.ScreenW - 24, game.ScreenH - 24
	menuListX                  = menuX + 216
	menuRowH                   = 17
	menuRows                   = 14 // rows shown at once
)

// drawMenu draws the items or shop screen over everything.
func (c *Crawl) drawMenu(dst *ebiten.Image, ctx *game.Context) {
	m, f, h := c.menu, ctx.Font, &c.run.hero
	gfx.FillRect(dst, 0, 0, game.ScreenW, game.ScreenH, pal.Fade(pal.Black, 0.6))
	gfx.Window(dst, menuX, menuY, menuW, menuH)

	title := fmt.Sprintf("%s · level %d", h.Class, h.Level)
	if m.shop {
		title = "The Merchant"
	}
	f.DrawShadow(dst, title, menuX+16, menuY+12, 2, pal.Yellow)
	gold := fmt.Sprintf("Gold %d", h.Gold)
	f.DrawShadow(dst, gold, menuX+menuW-16-f.Width(gold, 1), menuY+20, 1, pal.Yellow)

	// Left: the merchant, or the hero's stats.
	x, y := menuX+16, menuY+52
	if m.shop {
		if m.portra == nil {
			m.portra = gfx.Upload(proc.MerchantSprite(0).RGBA())
		}
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(5, 5)
		op.GeoM.Translate(float64(x+20), float64(y))
		dst.DrawImage(m.portra, op)
		f.DrawShadow(dst, "\"Finest wares", x+8, y+172, 1, pal.Tan)
		f.DrawShadow(dst, " in the dungeon!\"", x+8, y+188, 1, pal.Tan)
	} else {
		c.drawStats(dst, ctx, x, y)
	}

	// Right: the list.
	if m.sel < m.top {
		m.top = m.sel
	}
	if m.sel >= m.top+menuRows {
		m.top = m.sel - menuRows + 1
	}
	if m.top > 0 && m.rows[m.top-1].head && m.sel == m.top {
		m.top-- // keep a heading with its first row
	}
	lw := menuX + menuW - 16 - menuListX
	for i := m.top; i < min(len(m.rows), m.top+menuRows); i++ {
		row := m.rows[i]
		ry := menuY + 48 + (i-m.top)*menuRowH
		if row.head {
			f.DrawShadow(dst, row.label, menuListX, ry, 1, pal.Tan)
			continue
		}
		col := pal.Steel
		switch {
		case i == m.sel:
			gfx.FillRect(dst, menuListX-4, ry-1, lw+8, menuRowH, pal.Indigo)
			col = pal.White
		case row.act == nil:
			col = pal.Ash
		}
		f.DrawShadow(dst, row.label, menuListX+12, ry, 1, col)
		f.DrawShadow(dst, row.right, menuListX+lw-f.Width(row.right, 1), ry, 1, rowRightCol(row, i == m.sel))
	}
	if m.top > 0 {
		f.Draw(dst, "▲", menuListX+lw/2, menuY+36, 1, pal.Ash)
	}
	if m.top+menuRows < len(m.rows) {
		f.Draw(dst, "▼", menuListX+lw/2, menuY+48+menuRows*menuRowH, 1, pal.Ash)
	}

	// Bottom: about the chosen row, what just happened, and the keys.
	by := menuY + menuH - 62
	gfx.FillRect(dst, menuX+8, by-6, menuW-16, 1, pal.Indigo)
	f.DrawShadow(dst, m.rows[m.sel].about, menuX+16, by, 1, pal.Ice)
	f.DrawShadow(dst, m.note, menuX+16, by+18, 1, pal.Lime)
	f.DrawShadow(dst, c.menuHelp(), menuX+16, by+36, 1, pal.Ash)
}

func rowRightCol(row menuRow, sel bool) color.RGBA {
	switch {
	case row.act == nil:
		return pal.Ash
	case sel:
		return pal.Yellow
	}
	return pal.Tan
}

func (c *Crawl) menuHelp() string {
	m := c.menu
	row := m.rows[m.sel]
	switch {
	case m.shop && row.bag < 0 && row.act != nil:
		return "↑/↓ choose · Enter buy or sell · Esc leave"
	case m.shop:
		return "↑/↓ choose · Esc leave"
	case row.bag >= 0 && c.inBattle():
		return "↑/↓ choose · D drop · Esc back · Gear can't be changed in a battle"
	case row.bag >= 0:
		return "↑/↓ choose · Enter wear · D drop · Esc back"
	case row.act != nil:
		return "↑/↓ choose · Enter use · Esc back"
	}
	return "↑/↓ choose · Esc back"
}

// drawStats lists the hero's stats, with what their gear adds.
func (c *Crawl) drawStats(dst *ebiten.Image, ctx *game.Context, x, y int) {
	f, h := ctx.Font, &c.run.hero
	s := h.Stats()
	lines := []struct {
		name, value string
	}{
		{"HP", fmt.Sprintf("%d/%d", max(0, h.HP), s.MaxHP)},
		{"MP", fmt.Sprintf("%d/%d", h.MP, s.MaxMP)},
		{"XP", fmt.Sprintf("%d/%d", h.XP, h.NextXP())},
		{"ATK", fmt.Sprint(s.ATK)},
		{"DEF", fmt.Sprint(s.DEF)},
		{"Focus", fmt.Sprint(s.Focus)},
		{"Luck", fmt.Sprint(s.Luck)},
		{"Typing time", fmt.Sprintf("+%d%%", int(h.TimeBonus()*100+0.5)-100)},
		{"Dodge time", fmt.Sprintf("+%d%%", int(h.DodgeBonus()*100+0.5)-100)},
	}
	for i, l := range lines {
		ly := y + i*18
		f.DrawShadow(dst, l.name, x, ly, 1, pal.Steel)
		f.DrawShadow(dst, l.value, x+180-f.Width(l.value, 1), ly, 1, pal.Ice)
	}
	y += len(lines)*18 + 8
	for _, buff := range []struct {
		on   bool
		text string
	}{{h.Hourglass, "Hourglass ready"}, {h.Clarity, "Rune of Clarity ready"}} {
		if buff.on {
			f.DrawShadow(dst, buff.text, x, y, 1, pal.Cyan)
			y += 18
		}
	}
}
