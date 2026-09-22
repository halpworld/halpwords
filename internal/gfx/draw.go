package gfx

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/halpworld/halpwords/internal/pal"
)

// ArtScale is how many screen pixels one art pixel covers. World graphics are
// drawn on a 320×180 grid and scaled up; text is drawn at screen resolution.
const ArtScale = 2

// FillRect fills a rectangle with a solid colour.
func FillRect(dst *ebiten.Image, x, y, w, h int, c color.Color) {
	vector.FillRect(dst, float32(x), float32(y), float32(w), float32(h), c, false)
}

// Window draws a classic RPG dialogue window: a dark translucent panel with a
// light double border.
func Window(dst *ebiten.Image, x, y, w, h int) {
	FillRect(dst, x, y, w, h, pal.Black)
	FillRect(dst, x+1, y+1, w-2, h-2, pal.Ice)
	FillRect(dst, x+3, y+3, w-6, h-6, pal.Black)
	FillRect(dst, x+4, y+4, w-8, h-8, pal.Fade(pal.Night, 0.92))
	// Corner studs, for a bit of dungeon hardware.
	for _, p := range [][2]int{{x + 1, y + 1}, {x + w - 5, y + 1}, {x + 1, y + h - 5}, {x + w - 5, y + h - 5}} {
		FillRect(dst, p[0], p[1], 4, 4, pal.Tan)
		FillRect(dst, p[0]+1, p[1]+1, 2, 2, pal.Bronze)
	}
}

// Upload turns a procedurally generated image into an Ebitengine image.
func Upload(img *image.RGBA) *ebiten.Image {
	return ebiten.NewImageFromImage(img)
}

// DrawArt draws an art-resolution image scaled by ArtScale at screen (x, y).
func DrawArt(dst, img *ebiten.Image, x, y int) {
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(ArtScale, ArtScale)
	op.GeoM.Translate(float64(x), float64(y))
	dst.DrawImage(img, op)
}
