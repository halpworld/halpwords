package scene

import (
	"fmt"
	"image/color"
	"sort"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/pkg/words"
)

// grimoireSort is the order the Grimoire lists words in.
type grimoireSort int

const (
	sortWeakest grimoireSort = iota
	sortList
	sortAZ
	numSorts
)

var sortNames = [numSorts]string{"weakest first", "list order", "A to Z"}

// Grimoire shows what the player knows of each word: its Leitner box, how
// often it was right, how fast, and the words they struggle with most.
type Grimoire struct {
	bg      *ebiten.Image
	langs   []*words.Language
	li      int
	fixed   bool // opened during an adventure: the language can't change
	entries []words.Entry
	order   []int
	sum     words.Summary
	by      grimoireSort
	sel     int
	top     int
}

// NewGrimoire creates the Grimoire. With a language, it is opened over an
// adventure in that language and Esc goes back to it; otherwise it is
// opened from the title screen.
func NewGrimoire(ctx *game.Context, lang *words.Language) game.Scene {
	g := &Grimoire{bg: backdrop(6, 1.5)}
	if lang != nil {
		g.langs, g.fixed = []*words.Language{lang}, true
	} else {
		for _, l := range words.Languages {
			if len(ctx.ListsFor(l.Code)) > 0 {
				g.langs = append(g.langs, l)
			}
		}
	}
	g.load(ctx)
	return g
}

func (g *Grimoire) lang() *words.Language {
	if len(g.langs) == 0 {
		return nil
	}
	return g.langs[g.li]
}

// load reads the words for the language shown and sorts them.
func (g *Grimoire) load(ctx *game.Context) {
	g.entries, g.order, g.sel, g.top = nil, nil, 0, 0
	lang := g.lang()
	if lang == nil {
		return
	}
	mem := ctx.Profile.MemoryFor(lang.Code)
	seen := map[string]bool{}
	for _, e := range entriesFor(ctx, lang) {
		if k := words.Key(e); !seen[k] {
			seen[k] = true
			g.entries = append(g.entries, e)
		}
	}
	g.sum = mem.Summarize(g.entries)
	g.order = make([]int, len(g.entries))
	for i := range g.order {
		g.order[i] = i
	}
	switch g.by {
	case sortWeakest:
		weak := mem.Weakest(g.entries, len(g.entries))
		rank := map[int]int{}
		for r, i := range weak {
			rank[i] = r + 1
		}
		// Weak words first, then the rest by box: new words last.
		sort.SliceStable(g.order, func(a, b int) bool {
			ia, ib := g.order[a], g.order[b]
			ra, rb := rank[ia], rank[ib]
			switch {
			case ra > 0 && rb > 0:
				return ra < rb
			case ra > 0 || rb > 0:
				return ra > 0
			}
			ba, bb := mem.Box(g.entries[ia]), mem.Box(g.entries[ib])
			return ba > bb
		})
	case sortAZ:
		sort.SliceStable(g.order, func(a, b int) bool {
			return strings.ToLower(g.entries[g.order[a]].Prompt) < strings.ToLower(g.entries[g.order[b]].Prompt)
		})
	}
}

// Layout of the word list.
const (
	grX, grY, grW = 12, 128, game.ScreenW - 24
	grRowH        = 16
	grRows        = 9
)

// close goes back to the adventure or the title screen.
func (g *Grimoire) close(ctx *game.Context) {
	ctx.Sound.Play(audio.Back)
	if g.fixed {
		ctx.Pop()
		return
	}
	ctx.Replace(NewTitle(ctx))
}

// Update implements game.Scene.
func (g *Grimoire) Update(ctx *game.Context) error {
	n := len(g.order)
	move := func(by int) {
		if n == 0 {
			return
		}
		ctx.Sound.Play(audio.Blip)
		g.sel = max(0, min(n-1, g.sel+by))
	}
	switch {
	case input.Back() || input.Pressed(ebiten.KeyG):
		g.close(ctx)
	case input.Up():
		move(-1)
	case input.Down():
		move(1)
	case input.Repeat(ebiten.KeyPageUp):
		move(-grRows)
	case input.Repeat(ebiten.KeyPageDown):
		move(grRows)
	case input.Pressed(ebiten.KeyTab):
		ctx.Sound.Play(audio.Accent)
		g.by = (g.by + 1) % numSorts
		g.load(ctx)
	case !g.fixed && len(g.langs) > 1 && (input.Repeat(ebiten.KeyArrowLeft) || input.Repeat(ebiten.KeyArrowRight)):
		ctx.Sound.Play(audio.Blip)
		step := 1
		if input.Repeat(ebiten.KeyArrowLeft) {
			step = -1
		}
		g.li = (g.li + step + len(g.langs)) % len(g.langs)
		g.load(ctx)
	}
	if g.sel < g.top {
		g.top = g.sel
	}
	if g.sel >= g.top+grRows {
		g.top = g.sel - grRows + 1
	}
	return nil
}

// boxColors colour the boxes: new words, then box 1 (struggling) to box 5
// (mastered).
var boxColors = [words.Boxes + 1]color.RGBA{pal.Stone, pal.Rose, pal.Orange, pal.Yellow, pal.Lime, pal.Cyan}

// Draw implements game.Scene.
func (g *Grimoire) Draw(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	gfx.DrawArt(dst, g.bg, 0, 0)
	f.DrawShadow(dst, "Grimoire", 16, 8, 2, pal.Yellow)
	lang := g.lang()
	if lang == nil {
		f.DrawCentered(dst, "No word lists found.", game.ScreenW/2, 170, 1, pal.Rose)
		return
	}
	name := lang.Name
	if !g.fixed && len(g.langs) > 1 {
		name = "◄ " + name + " ►"
	}
	f.DrawShadow(dst, name, game.ScreenW-16-f.Width(name, 2), 8, 2, pal.Tan)

	// Summary: how many words are in each box, and how answers go.
	s := g.sum
	gfx.Window(dst, grX, 40, grW, 84)
	x, w := grX+14, grW-28
	head := fmt.Sprintf("%d of %d words mastered", s.Mastered(), s.Words)
	if s.Seen > 0 {
		head += fmt.Sprintf("   ·   right %d%%", int(s.Accuracy()*100+0.5))
	}
	if s.Timed > 0 {
		head += fmt.Sprintf("   ·   %.1fs an answer", s.AvgSecs())
	}
	f.DrawShadow(dst, head, x, 48, 1, pal.White)
	bx := x
	for b := words.Boxes; b >= 0; b-- {
		n := s.InBox[b]
		if n == 0 || s.Words == 0 {
			continue
		}
		bw := max(2, w*n/s.Words)
		if b == 0 {
			bw = x + w - bx // the new words take what is left
		}
		gfx.FillRect(dst, bx, 67, bw, 10, boxColors[b])
		bx += bw
	}
	legend := fmt.Sprintf("mastered %d · box 4: %d · 3: %d · 2: %d · 1: %d · new %d",
		s.InBox[5], s.InBox[4], s.InBox[3], s.InBox[2], s.InBox[1], s.InBox[0])
	f.DrawShadow(dst, legend, x, 82, 1, pal.Steel)
	switch m := s.CommonMistake(); {
	case s.Seen == 0:
		f.DrawShadow(dst, "Words you meet in the dungeon and in practice appear here.", x, 100, 1, pal.Tan)
	case m != words.NoMistake:
		f.DrawShadow(dst, "Your most common slip: "+m.String()+".", x, 100, 1, pal.Orange)
	default:
		f.DrawShadow(dst, "No slips yet. Well done!", x, 100, 1, pal.Lime)
	}

	// The words.
	gfx.Window(dst, grX, grY, grW, grRows*grRowH+52)
	cols := [...]int{grX + 14, grX + 214, grX + 440, grX + 506, grX + 560}
	hy := grY + 8
	for i, h := range []string{"English", lang.Name, "Box", "Right", "Time"} {
		f.DrawShadow(dst, h, cols[i], hy, 1, pal.Tan)
	}
	mem := ctx.Profile.MemoryFor(lang.Code)
	for r := 0; r < grRows && g.top+r < len(g.order); r++ {
		i := g.top + r
		e := g.entries[g.order[i]]
		ry := hy + 20 + r*grRowH
		col := pal.Steel
		if i == g.sel {
			gfx.FillRect(dst, grX+6, ry-1, grW-12, grRowH, pal.Indigo)
			col = pal.White
		}
		f.DrawShadow(dst, clip(f, e.Prompt, cols[1]-cols[0]-10), cols[0], ry, 1, col)
		f.DrawShadow(dst, clip(f, e.Answers[0], cols[2]-cols[1]-10), cols[1], ry, 1, col)
		c := mem.Card(e)
		box := 0
		if c != nil {
			box = c.Box
		}
		for p := 1; p <= words.Boxes; p++ {
			pc, glyph := pal.Stone, "□"
			if p <= box {
				pc, glyph = boxColors[box], "■"
			}
			f.Draw(dst, glyph, cols[2]+(p-1)*10, ry, 1, pc)
		}
		if c == nil {
			f.DrawShadow(dst, "new", cols[3], ry, 1, pal.Ash)
			continue
		}
		f.DrawShadow(dst, fmt.Sprintf("%d%%", int(c.Accuracy()*100+0.5)), cols[3], ry, 1, col)
		if c.Timed > 0 {
			f.DrawShadow(dst, fmt.Sprintf("%.1fs", c.AvgSecs()), cols[4], ry, 1, col)
		}
	}
	if g.top > 0 {
		f.Draw(dst, "▲", grX+grW-24, hy, 1, pal.Ash)
	}
	if g.top+grRows < len(g.order) {
		f.Draw(dst, "▼", grX+grW-24, hy+20+(grRows-1)*grRowH, 1, pal.Ash)
	}
	g.drawSelected(dst, ctx, mem)

	help := "↑/↓ scroll · Tab sort: " + sortNames[g.by] + " · Esc back"
	if !g.fixed && len(g.langs) > 1 {
		help = "←/→ language · " + help
	}
	f.DrawShadow(dst, help, 8, game.ScreenH-20, 1, pal.Ash)
}

// drawSelected describes the chosen word under the list.
func (g *Grimoire) drawSelected(dst *ebiten.Image, ctx *game.Context, mem *words.Memory) {
	if len(g.order) == 0 {
		return
	}
	f := ctx.Font
	e := g.entries[g.order[g.sel]]
	y := grY + 34 + grRows*grRowH
	c := mem.Card(e)
	var text string
	switch {
	case c == nil:
		text = "Not met yet."
	case c.Box == words.Boxes:
		text = fmt.Sprintf("Mastered! Answered %d times.", c.Seen)
	default:
		text = fmt.Sprintf("Answered %d times, %d perfect, %d missed.", c.Seen, c.Perfect, c.Misses)
		worst, n := words.NoMistake, 0
		for m, k := range c.Mistakes {
			if k > n {
				worst, n = words.Mistake(m), k
			}
		}
		if worst != words.NoMistake {
			text += " Watch out for " + worst.String() + "."
		}
	}
	f.DrawShadow(dst, text, grX+grW-14-f.Width(text, 1), y, 1, pal.Ice)
}

// clip shortens s with "…" to fit in width at scale 1.
func clip(f *gfx.Font, s string, width int) string {
	if f.Width(s, 1) <= width {
		return s
	}
	rs := []rune(s)
	for len(rs) > 0 && f.Width(string(rs)+"…", 1) > width {
		rs = rs[:len(rs)-1]
	}
	return string(rs) + "…"
}
