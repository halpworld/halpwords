package scene

import (
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/input"
)

// typed reports whether the letter r was typed this tick, in either
// case. It reads the typed character, so it follows the keyboard's
// layout: on a German (QWERTZ) keyboard the key marked Y is where a US
// keyboard has Z. Only when nothing was typed this tick (a platform
// without typed characters, or a test) does it fall back to the key
// where a US keyboard has the letter.
func typed(ctx *game.Context, r rune, key ebiten.Key) bool {
	if ctx.Input != nil && len(ctx.Input.Chars) > 0 {
		for _, c := range ctx.Input.Chars {
			if unicode.ToLower(c) == r {
				return true
			}
		}
		return false
	}
	return input.Pressed(key)
}

// typedYes and typedNo are Y and N in a yes-or-no question.
func typedYes(ctx *game.Context) bool { return typed(ctx, 'y', ebiten.KeyY) }
func typedNo(ctx *game.Context) bool  { return typed(ctx, 'n', ebiten.KeyN) }
