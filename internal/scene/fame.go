package scene

import (
	"fmt"
	"strings"
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

// fameModes are the Hall of Fame's tables for each language.
var fameModes = []compete.Mode{compete.Hardcore, compete.Daily}

// HallOfFame lists the best Hardcore and Daily Dungeon runs in each
// language, and checks friends' share codes.
type HallOfFame struct {
	bg   *ebiten.Image
	li   int
	mi   int
	mark int // the place to highlight, from 1, or 0

	checking bool // typing a friend's share code
	code     []rune
	result   []logLine
}

// NewHallOfFame creates the Hall of Fame screen.
func NewHallOfFame(*game.Context) game.Scene { return &HallOfFame{bg: backdrop(8, 1.4)} }

// hallAt opens the Hall of Fame on a table, with place (from 1) marked.
func hallAt(mode compete.Mode, lang *words.Language, place int) *HallOfFame {
	h := &HallOfFame{bg: backdrop(8, 1.4), mark: place}
	for i, l := range words.Languages {
		if l == lang {
			h.li = i
		}
	}
	for i, m := range fameModes {
		if m == mode {
			h.mi = i
		}
	}
	return h
}

func (h *HallOfFame) lang() *words.Language { return words.Languages[h.li] }

// Update implements game.Scene.
func (h *HallOfFame) Update(ctx *game.Context) error {
	if h.checking {
		h.updateCheck(ctx)
		return nil
	}
	n := len(words.Languages)
	switch {
	case input.Back():
		ctx.Sound.Play(audio.Back)
		ctx.Replace(NewTitle(ctx))
	case input.Repeat(ebiten.KeyArrowLeft) || input.Repeat(ebiten.KeyA):
		ctx.Sound.Play(audio.Blip)
		h.li, h.mark = (h.li+n-1)%n, 0
	case input.Repeat(ebiten.KeyArrowRight) || input.Repeat(ebiten.KeyD):
		ctx.Sound.Play(audio.Blip)
		h.li, h.mark = (h.li+1)%n, 0
	case input.Pressed(ebiten.KeyTab) || input.Up() || input.Down():
		ctx.Sound.Play(audio.Accent)
		h.mi, h.mark = (h.mi+1)%len(fameModes), 0
	case input.Pressed(ebiten.KeyC) || input.Confirm():
		ctx.Sound.Play(audio.Select)
		h.checking, h.code, h.result = true, nil, nil
		ctx.Input.Chars = ctx.Input.Chars[:0]
	}
	return nil
}

// updateCheck reads a friend's share code and says what it holds.
func (h *HallOfFame) updateCheck(ctx *game.Context) {
	if input.Back() {
		ctx.Sound.Play(audio.Back)
		h.checking = false
		return
	}
	before := len(h.code)
	h.code = typeCode(ctx, h.code, maxCodeLen)
	if len(h.code) != before {
		h.result = nil
	}
	if input.Confirm() && len(h.code) > 0 {
		h.result = checkCode(ctx, string(h.code))
		if h.result[0].col == pal.Lime {
			ctx.Sound.Play(audio.Perfect)
		} else {
			ctx.Sound.Play(audio.Wrong)
		}
	}
}

// checkCode describes a friend's share code, and how it compares with the
// player's best.
func checkCode(ctx *game.Context, code string) []logLine {
	s, err := compete.ParseShare(code)
	if err != nil {
		return []logLine{{"✗ " + err.Error(), pal.Rose}}
	}
	lang, _ := words.Lookup(s.Lang)
	mode, run := compete.Hardcore, "seed "+compete.SeedCode(s.Seed)
	if s.Daily {
		mode = compete.Daily
		run = fmt.Sprintf("Daily Dungeon, %d %s", s.Day, s.Month.String()[:3])
	}
	lines := []logLine{
		{"✓ The code checks out!", pal.Lime},
		{fmt.Sprintf("%s · %s · floor %d · %s points", lang.Name, run, s.Floor, groupDigits(s.Score)), pal.White},
	}
	best := ctx.Profile.Fame.Best(compete.TableKey(mode, lang.Code))
	switch {
	case best == 0:
		lines = append(lines, logLine{"You have no " + mode.String() + " score in " + lang.Name + " yet.", pal.Tan})
	case best > s.Score:
		lines = append(lines, logLine{fmt.Sprintf("Your best is %s: you're ahead!", groupDigits(best)), pal.Yellow})
	case best == s.Score:
		lines = append(lines, logLine{"Your best is exactly the same!", pal.Yellow})
	default:
		lines = append(lines, logLine{fmt.Sprintf("Your best is %s: time for a rematch!", groupDigits(best)), pal.Orange})
	}
	if !s.Daily {
		lines = append(lines, logLine{"Play it: New Adventure → Seed Challenge → " + compete.SeedCode(s.Seed), pal.Tan})
	}
	return lines
}

// groupDigits writes n with thin groups of three digits: 18,450.
func groupDigits(n int) string {
	s := fmt.Sprint(n)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteRune(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Draw implements game.Scene.
func (h *HallOfFame) Draw(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	gfx.DrawArt(dst, h.bg, 0, 0)
	f.DrawCentered(dst, "Hall of Fame", cx, 8, 3, pal.Yellow)
	mode := fameModes[h.mi]
	f.DrawCentered(dst, "◄ "+h.lang().Name+" ►   ·   "+mode.String(), cx, 60, 1, pal.Tan)

	const x, w, rowH = 24, game.ScreenW - 48, 20
	y := 82
	gfx.Window(dst, x, y, w, rowH*compete.FameSize+34)
	cols := [...]int{x + 16, x + 56, x + 220, x + 316, x + 380, x + 478}
	for i, head := range []string{"#", "Name", "Class", "Floor", "Score", "Date"} {
		f.DrawShadow(dst, head, cols[i], y+8, 1, pal.Tan)
	}
	table := ctx.Profile.Fame.Table(compete.TableKey(mode, h.lang().Code))
	if len(table) == 0 {
		f.DrawCentered(dst, "No runs yet. Will yours be the first?", cx, y+100, 1, pal.Ash)
	}
	for i, fm := range table {
		ry := y + 28 + i*rowH
		col := pal.Steel
		if i == 0 {
			col = pal.Yellow
		}
		if i+1 == h.mark {
			gfx.FillRect(dst, x+6, ry-2, w-12, rowH-2, pal.Indigo)
			col = pal.White
		}
		for k, v := range []string{fmt.Sprint(i + 1), fm.Name, fm.Class, fmt.Sprint(fm.Floor), groupDigits(fm.Score), fm.Date} {
			f.DrawShadow(dst, clip(f, v, 150), cols[k], ry, 1, col)
		}
	}

	help := "←/→ language · Tab table · C check a friend's code · Esc back"
	if h.checking {
		h.drawCheck(dst, ctx)
		help = "Type the share code · Enter check · Esc close"
	}
	f.DrawShadow(dst, help, 8, game.ScreenH-20, 1, pal.Ash)
}

func (h *HallOfFame) drawCheck(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	const w, hh = 560, 170
	x, y := game.ScreenW/2-w/2, game.ScreenH/2-hh/2
	gfx.FillRect(dst, 0, 0, game.ScreenW, game.ScreenH, pal.Fade(pal.Black, 0.5))
	gfx.Window(dst, x, y, w, hh)
	f.DrawCentered(dst, "Check a friend's code", x+w/2, y+10, 2, pal.Yellow)
	f.DrawCentered(dst, "Like HW-FR-0922-F12-18450-K7QX:", x+w/2, y+42, 1, pal.Tan)
	drawTyped(dst, ctx, string(h.code), x+w/2, y+58, w-40, 2, h.result == nil)
	for i, l := range h.result {
		f.DrawCentered(dst, l.text, x+w/2, y+98+i*16, 1, l.col)
	}
}

// today is the date runs are recorded with.
func today() string { return time.Now().Format(time.DateOnly) }
