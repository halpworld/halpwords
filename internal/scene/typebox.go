package scene

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/typing"
	"github.com/halpworld/halpwords/internal/words"
)

// typeInto feeds this tick's typing into f: characters, Backspace, Tab for
// accents and F2 for Greek keys. It reports whether Enter was pressed with
// something typed.
func typeInto(ctx *game.Context, f *typing.Field) bool {
	if input.Pressed(ebiten.KeyF2) && f.Lang.Script == words.ScriptGreek {
		f.Greek = !f.Greek
	}
	for _, r := range ctx.Input.Chars {
		f.Type(r)
	}
	if input.Repeat(ebiten.KeyBackspace) {
		f.Backspace()
	}
	if input.Pressed(ebiten.KeyTab) {
		f.CycleAccent()
	}
	return input.Confirm() && f.Len() > 0
}

// drawTyped draws typed text centred on cx, with an underline across width
// and a blinking cursor when cursor is set.
func drawTyped(dst *ebiten.Image, ctx *game.Context, text string, cx, y, width, maxScale int, cursor bool) {
	f := ctx.Font
	sc := f.FitScale(text+"_", width, maxScale)
	gfx.FillRect(dst, cx-width/2, y+16*maxScale+2, width, 2, pal.Indigo)
	y += 16 * (maxScale - sc) // keep the baseline when the text shrinks
	tw := f.Width(text, sc)
	tx := cx - tw/2
	f.DrawShadow(dst, text, tx, y, sc, pal.Ice)
	if cursor && ctx.Tick/16%2 == 0 {
		gfx.FillRect(dst, tx+tw+2, y+2, 4*sc, 14*sc, pal.Yellow)
	}
}

// bar draws a horizontal gauge filled to frac.
func bar(dst *ebiten.Image, x, y, w, h int, frac float64, fg, bg color.Color) {
	frac = max(0, min(1, frac))
	gfx.FillRect(dst, x, y, w, h, pal.Black)
	gfx.FillRect(dst, x+1, y+1, w-2, h-2, bg)
	gfx.FillRect(dst, x+1, y+1, int(float64(w-2)*frac), h-2, fg)
}
