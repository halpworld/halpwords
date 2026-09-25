package scene

import (
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/audio"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/pkg/compete"
)

// modeChoice is a way to play, offered by the New Adventure screen.
type modeChoice struct {
	name  string
	about [2]string
	mode  compete.Mode
	seed  bool // asks for a seed code
}

var modeChoices = []modeChoice{
	{"Adventure", [2]string{"Save Shrines keep your progress.", "If you fall, you wake at the last one."}, compete.Adventure, false},
	{"Hardcore", [2]string{"One life, no shrines, and a score.", "Your best runs go in the Hall of Fame."}, compete.Hardcore, false},
	{"Daily Dungeon", [2]string{"Today's Hardcore dungeon: everyone with", "the same word lists gets the same one."}, compete.Daily, false},
	{"Seed Challenge", [2]string{"Play a friend's Hardcore dungeon:", "type its seed code or share code."}, compete.Hardcore, true},
}

// NewGame chooses how to play a new adventure.
type NewGame struct {
	bg  *ebiten.Image
	sel int
	// entering is set while a seed code is typed.
	entering bool
	code     []rune
	err      string
}

// NewNewGame creates the mode picker.
func NewNewGame(*game.Context) game.Scene { return &NewGame{bg: backdrop(3, 1.4)} }

// maxCodeLen is the longest code the seed box takes: a share code.
const maxCodeLen = 32

// Update implements game.Scene.
func (n *NewGame) Update(ctx *game.Context) error {
	if n.entering {
		n.updateCode(ctx)
		return nil
	}
	switch {
	case input.Back():
		ctx.Sound.Play(audio.Back)
		ctx.Replace(NewTitle(ctx))
	case input.Up():
		ctx.Sound.Play(audio.Blip)
		n.sel = (n.sel + len(modeChoices) - 1) % len(modeChoices)
	case input.Down():
		ctx.Sound.Play(audio.Blip)
		n.sel = (n.sel + 1) % len(modeChoices)
	case input.Confirm() || input.Pressed(ebiten.KeySpace):
		ctx.Sound.Play(audio.Select)
		mc := modeChoices[n.sel]
		if mc.seed {
			n.entering, n.err = true, ""
			ctx.Input.Chars = ctx.Input.Chars[:0]
			return nil
		}
		ctx.Replace(NewAdventure(ctx, runSetup{mode: mc.mode}))
	}
	return nil
}

// updateCode reads a seed code as it is typed.
func (n *NewGame) updateCode(ctx *game.Context) {
	if input.Back() {
		ctx.Sound.Play(audio.Back)
		n.entering = false
		return
	}
	n.code = typeCode(ctx, n.code, maxCodeLen)
	if input.Confirm() && len(n.code) > 0 {
		seed, err := compete.SeedFromCode(string(n.code))
		if err != nil {
			ctx.Sound.Play(audio.Wrong)
			n.err = err.Error()
			return
		}
		ctx.Sound.Play(audio.Select)
		ctx.Replace(NewAdventure(ctx, runSetup{mode: compete.Hardcore, seed: seed, seeded: true}))
	}
}

// typeCode adds this tick's typing to a code: letters, digits and dashes,
// up to max characters. Backspace takes one off.
func typeCode(ctx *game.Context, code []rune, most int) []rune {
	for _, r := range ctx.Input.Chars {
		if (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-') && r < 0x80 && len(code) < most {
			code = append(code, unicode.ToUpper(r))
			ctx.Sound.Play(audio.Key)
		}
	}
	if input.Repeat(ebiten.KeyBackspace) && len(code) > 0 {
		code = code[:len(code)-1]
		ctx.Sound.Play(audio.Erase)
	}
	return code
}

// Draw implements game.Scene.
func (n *NewGame) Draw(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	cx := game.ScreenW / 2
	gfx.DrawArt(dst, n.bg, 0, 0)
	f.DrawCentered(dst, "How will you play?", cx, 24, 3, pal.Yellow)

	const w, rowH = 320, 34
	x, y := cx-w/2, 84
	gfx.Window(dst, x, y, w, rowH*len(modeChoices)+20)
	for i, mc := range modeChoices {
		ry := y + 14 + i*rowH
		col := pal.Steel
		if i == n.sel {
			col = pal.White
			if ctx.Tick/20%2 == 0 {
				f.Draw(dst, "►", x+18, ry, 2, pal.Yellow)
			}
		}
		f.DrawShadow(dst, mc.name, x+48, ry, 2, col)
	}
	about := modeChoices[n.sel].about
	ay := y + rowH*len(modeChoices) + 32
	gfx.Window(dst, cx-220, ay, 440, 52)
	for k, line := range about {
		f.DrawCentered(dst, line, cx, ay+10+k*16, 1, pal.Ice)
	}
	help := "↑/↓ choose   Enter next   Esc back"
	if n.entering {
		n.drawCodeBox(dst, ctx)
		help = "Type the code   Enter play   Esc back"
	}
	f.DrawShadow(dst, help, 8, game.ScreenH-20, 1, pal.Ash)
}

func (n *NewGame) drawCodeBox(dst *ebiten.Image, ctx *game.Context) {
	f := ctx.Font
	const w, h = 440, 120
	x, y := game.ScreenW/2-w/2, game.ScreenH/2-h/2
	gfx.FillRect(dst, 0, 0, game.ScreenW, game.ScreenH, pal.Fade(pal.Black, 0.5))
	gfx.Window(dst, x, y, w, h)
	f.DrawCentered(dst, "Seed Challenge", x+w/2, y+10, 2, pal.Yellow)
	f.DrawCentered(dst, "A seed code (7K3QZP) or a friend's share code:", x+w/2, y+44, 1, pal.Tan)
	drawTyped(dst, ctx, string(n.code), x+w/2, y+62, w-40, 2, true)
	if n.err != "" {
		f.DrawCentered(dst, n.err, x+w/2, y+h-20, 1, pal.Rose)
	}
}
