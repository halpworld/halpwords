package scene

import (
	"fmt"
	"image/color"
	"math/rand/v2"
	"strings"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/combat"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/proc"
	"github.com/halpworld/halpwords/internal/typing"
	"github.com/halpworld/halpwords/internal/words"
)

// Practice is a stand-alone spelling drill. It exercises the typing, grading
// and scoring code that battles will use.
type Practice struct {
	bg    *ebiten.Image
	rng   *rand.Rand
	langs []*words.Language // languages that have at least one list
	li    int

	entries []words.Entry
	cur     int
	field   *typing.Field
	started uint64 // tick the current word appeared

	showing bool // showing the result of the last answer
	result  words.Result
	typed   string
	taken   float64
	streak  int
	best    int
}

// NewPractice creates the practice screen.
func NewPractice(ctx *game.Context) game.Scene {
	p := &Practice{bg: backdrop(7, 1.6), rng: proc.NewRand(ctx.Tick + 1)}
	for _, l := range words.Languages {
		if len(ctx.ListsFor(l.Code)) > 0 {
			p.langs = append(p.langs, l)
		}
	}
	p.setLanguage(ctx, 0)
	return p
}

func (p *Practice) lang() *words.Language { return p.langs[p.li] }

func (p *Practice) setLanguage(ctx *game.Context, i int) {
	p.li = (i + len(p.langs)) % len(p.langs)
	p.entries = p.entries[:0]
	for _, l := range ctx.ListsFor(p.lang().Code) {
		p.entries = append(p.entries, l.Entries...)
	}
	p.field = typing.NewField(p.lang())
	p.streak = 0
	p.cur = -1
	p.next(ctx)
}

func (p *Practice) next(ctx *game.Context) {
	n := p.rng.IntN(len(p.entries))
	if n == p.cur && len(p.entries) > 1 {
		n = (n + 1) % len(p.entries)
	}
	p.cur = n
	p.field.Reset()
	p.showing = false
	p.started = ctx.Tick
}

// Update implements game.Scene.
func (p *Practice) Update(ctx *game.Context) error {
	if input.Back() {
		ctx.Replace(NewTitle(ctx))
		return nil
	}
	if p.showing {
		if input.Confirm() {
			p.next(ctx)
		}
		return nil
	}
	switch {
	case input.Pressed(ebiten.KeyArrowLeft):
		p.setLanguage(ctx, p.li-1)
		return nil
	case input.Pressed(ebiten.KeyArrowRight):
		p.setLanguage(ctx, p.li+1)
		return nil
	case input.Pressed(ebiten.KeyF2) && p.lang().Script == words.ScriptGreek:
		p.field.Greek = !p.field.Greek
	}
	for _, r := range ctx.Input.Chars {
		p.field.Type(r)
	}
	if input.Repeat(ebiten.KeyBackspace) {
		p.field.Backspace()
	}
	if input.Pressed(ebiten.KeyTab) {
		p.field.CycleAccent()
	}
	if input.Confirm() && p.field.Len() > 0 {
		e := p.entries[p.cur]
		p.typed = p.field.Text()
		p.taken = float64(ctx.Tick-p.started) / float64(ebiten.TPS())
		p.result = words.Grade(p.typed, e, p.lang(), p.lang().Defaults, p.field.UsedBackspace)
		p.showing = true
		if p.result.Tier >= words.Correct {
			p.streak++
			p.best = max(p.best, p.streak)
		} else {
			p.streak = 0
		}
	}
	return nil
}

var tierColor = map[words.Tier]color.RGBA{
	words.Perfect:    pal.Lime,
	words.Correct:    pal.Green,
	words.AccentSlip: pal.Cyan,
	words.Graze:      pal.Orange,
	words.Miss:       pal.Rose,
}

// Draw implements game.Scene.
func (p *Practice) Draw(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	gfx.DrawArt(dst, p.bg, 0, 0)

	// Header: language selector and streak.
	f.DrawCentered(dst, "◄ "+p.lang().Name+" ►", cx, 12, 2, pal.Yellow)
	f.DrawShadow(dst, fmt.Sprintf("Streak %d", p.streak), 12, 12, 1, pal.Ice)
	f.DrawShadow(dst, fmt.Sprintf("Best %d", p.best), 12, 30, 1, pal.Steel)
	if len(ctx.ListErrors) > 0 {
		f.DrawShadow(dst, "Word list error: "+ctx.ListErrors[0], 12, game.ScreenH-38, 1, pal.Rose)
	}

	// Main window.
	const wx, wy, ww, wh = 40, 48, game.ScreenW - 80, 204
	gfx.Window(dst, wx, wy, ww, wh)
	e := p.entries[p.cur]
	f.DrawCentered(dst, "Translate into "+p.lang().Name+":", cx, wy+14, 1, pal.Steel)
	sc := f.FitScale(e.Prompt, ww-40, 3)
	f.DrawCentered(dst, e.Prompt, cx, wy+34, sc, pal.White)

	// Input line.
	text := p.field.Text()
	if p.showing {
		text = p.typed
	}
	sc = f.FitScale(text+"_", ww-40, 3)
	lineY := wy + 88
	gfx.FillRect(dst, wx+20, lineY+16*sc+4, ww-40, 2, pal.Indigo)
	tw := f.Width(text, sc)
	tx := cx - tw/2
	f.DrawShadow(dst, text, tx, lineY, sc, pal.Ice)
	if !p.showing && ctx.Tick/16%2 == 0 {
		gfx.FillRect(dst, tx+tw+2, lineY+2, 4*sc, 14*sc, pal.Yellow)
	}

	if p.showing {
		r := p.result
		f.DrawCentered(dst, r.Tier.String()+"!", cx, wy+148, 2, tierColor[r.Tier])
		info := fmt.Sprintf("Answer: %s", r.Expected)
		if r.Tier >= words.Graze {
			speed := combat.Speed(utf8.RuneCountInString(r.Expected), p.taken)
			power := combat.Accuracy(r.Tier) * speed
			info += fmt.Sprintf("    %.1fs   speed ×%.1f   power %d%%", p.taken, speed, int(power*100))
		}
		f.DrawCentered(dst, info, cx, wy+180, 1, pal.Ice)
	} else {
		secs := float64(ctx.Tick-p.started) / float64(ebiten.TPS())
		f.DrawCentered(dst, fmt.Sprintf("%.1fs", secs), cx, wy+160, 1, pal.Ash)
	}

	p.drawHelp(dst, ctx)
}

func (p *Practice) drawHelp(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	y := 262
	if p.showing {
		f.DrawCentered(dst, "Enter: next word    Esc: menu", game.ScreenW/2, y+40, 1, pal.Steel)
		return
	}
	if p.field.Greek {
		gfx.Window(dst, 40, y-6, game.ScreenW-80, 98)
		var row1, row2 strings.Builder
		for i, k := range typing.BetaCodeChart {
			b := &row1
			if i >= 12 {
				b = &row2
			}
			fmt.Fprintf(b, "%c %c  ", k.Key, k.Greek)
		}
		f.DrawCentered(dst, row1.String(), game.ScreenW/2, y+4, 1, pal.Ice)
		f.DrawCentered(dst, row2.String(), game.ScreenW/2, y+20, 1, pal.Ice)
		f.DrawCentered(dst, "after a vowel:  ) ἀ   ( ἁ   / ά   \\ ὰ   = ᾶ   | ᾳ   + ϊ", game.ScreenW/2, y+38, 1, pal.Tan)
		f.DrawCentered(dst, "Tab: cycle accent   F2: Greek keys off", game.ScreenW/2, y+54, 1, pal.Tan)
		f.DrawCentered(dst, "Enter: check   ←/→: language   Esc: menu", game.ScreenW/2, y+70, 1, pal.Steel)
		return
	}
	help := "Enter: check   Backspace: fix   ←/→: language   Esc: menu"
	if cycle, _, ok := p.lang().AccentCycle('e'); ok {
		hint := strings.Join(strings.Split(string(cycle), ""), " → ")
		f.DrawCentered(dst, "Tab after a letter adds an accent:  "+hint, game.ScreenW/2, y+22, 1, pal.Tan)
	}
	if p.lang().Script == words.ScriptGreek {
		help = "F2: Greek keys on   " + help
	}
	f.DrawCentered(dst, help, game.ScreenW/2, y+40, 1, pal.Steel)
}
