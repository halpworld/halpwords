// Package scene contains the game's screens.
package scene

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/pkg/proc"
)

// stoneRamp runs from mortar to highlight for grey dungeon stone.
var stoneRamp = []color.RGBA{pal.Black, pal.Night, pal.Slate, pal.Granite, pal.Stone, pal.Ash}

// backdrop generates a vignetted brick wall covering the whole screen.
func backdrop(seed uint64, vignette float64) *ebiten.Image {
	img := proc.BrickWall(320, 180, stoneRamp, seed)
	proc.Vignette(img, vignette)
	return gfx.Upload(img)
}
