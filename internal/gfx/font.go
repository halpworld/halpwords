// Package gfx draws things with Ebitengine: text, windows, particles and
// uploads of procedurally generated images.
package gfx

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/text/unicode/norm"

	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/unifont"
)

// LineHeight is the height of a line of text at scale 1.
const LineHeight = unifont.Height

// Font renders Unifont text at integer scales so pixels stay crisp.
type Font struct {
	face  *unifont.Face
	cache map[rune]*ebiten.Image
}

// NewFont wraps a parsed Unifont face.
func NewFont(face *unifont.Face) *Font {
	return &Font{face: face, cache: map[rune]*ebiten.Image{}}
}

func (f *Font) glyph(r rune) (*ebiten.Image, int) {
	g := f.face.Glyph(r)
	if g == nil {
		return nil, 0
	}
	if img, ok := f.cache[r]; ok {
		return img, g.Width
	}
	rgba := image.NewRGBA(image.Rect(0, 0, g.Width, unifont.Height))
	for y := 0; y < unifont.Height; y++ {
		for x := 0; x < g.Width; x++ {
			if g.Set(x, y) {
				rgba.SetRGBA(x, y, color.RGBA{0xff, 0xff, 0xff, 0xff})
			}
		}
	}
	img := ebiten.NewImageFromImage(rgba)
	f.cache[r] = img
	return img, g.Width
}

// Width returns the width of s in screen pixels at the given scale.
func (f *Font) Width(s string, scale int) int {
	return f.face.Width(norm.NFC.String(s)) * scale
}

// Draw draws s with its top-left corner at (x, y).
func (f *Font) Draw(dst *ebiten.Image, s string, x, y, scale int, c color.Color) {
	op := &ebiten.DrawImageOptions{}
	for _, r := range norm.NFC.String(s) {
		img, w := f.glyph(r)
		if img == nil {
			continue
		}
		op.GeoM.Reset()
		op.GeoM.Scale(float64(scale), float64(scale))
		op.GeoM.Translate(float64(x), float64(y))
		op.ColorScale.Reset()
		op.ColorScale.ScaleWithColor(c)
		dst.DrawImage(img, op)
		x += w * scale
	}
}

// DrawShadow draws s with a one-pixel (times scale) drop shadow.
func (f *Font) DrawShadow(dst *ebiten.Image, s string, x, y, scale int, c color.Color) {
	f.Draw(dst, s, x+scale, y+scale, scale, pal.Black)
	f.Draw(dst, s, x, y, scale, c)
}

// DrawOutline draws s with a dark outline all round, for titles.
func (f *Font) DrawOutline(dst *ebiten.Image, s string, x, y, scale int, c, outline color.Color) {
	for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}, {-1, -1}, {1, -1}, {-1, 1}, {1, 1}} {
		f.Draw(dst, s, x+d[0]*scale, y+d[1]*scale, scale, outline)
	}
	f.Draw(dst, s, x, y, scale, c)
}

// DrawCentered draws s with a shadow, horizontally centred on cx.
func (f *Font) DrawCentered(dst *ebiten.Image, s string, cx, y, scale int, c color.Color) {
	f.DrawShadow(dst, s, cx-f.Width(s, scale)/2, y, scale, c)
}

// FitScale returns the largest scale in [1, maxScale] at which s fits in width.
func (f *Font) FitScale(s string, width, maxScale int) int {
	for sc := maxScale; sc > 1; sc-- {
		if f.Width(s, sc) <= width {
			return sc
		}
	}
	return 1
}
