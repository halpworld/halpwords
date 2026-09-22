package proc

import (
	"image"
	"image/color"

	"github.com/halpworld/halpwords/internal/pal"
)

// Transparent is the palette index for see-through pixels.
const Transparent = 255

// Indexed is an image whose pixels are positions in pal.All. The dungeon
// renderer shades these with lookup tables, so everything stays in the
// palette.
type Indexed struct {
	W, H int
	Pix  []uint8
	// Glow marks pixels that ignore darkness (flames, runes, eyes). It is nil
	// when nothing glows.
	Glow []bool
}

// NewIndexed returns a fully transparent image.
func NewIndexed(w, h int) *Indexed {
	m := &Indexed{W: w, H: h, Pix: make([]uint8, w*h)}
	for i := range m.Pix {
		m.Pix[i] = Transparent
	}
	return m
}

// In reports whether (x, y) is inside the image.
func (m *Indexed) In(x, y int) bool { return x >= 0 && y >= 0 && x < m.W && y < m.H }

// At returns the palette index at (x, y), or Transparent outside the image.
func (m *Indexed) At(x, y int) uint8 {
	if !m.In(x, y) {
		return Transparent
	}
	return m.Pix[y*m.W+x]
}

// Set paints (x, y) with c, which should be a palette colour.
func (m *Indexed) Set(x, y int, c color.RGBA) {
	if m.In(x, y) {
		m.Pix[y*m.W+x] = pal.Index(c)
	}
}

// SetGlow paints (x, y) with c and makes it glow.
func (m *Indexed) SetGlow(x, y int, c color.RGBA) {
	if !m.In(x, y) {
		return
	}
	if m.Glow == nil {
		m.Glow = make([]bool, m.W*m.H)
	}
	m.Set(x, y, c)
	m.Glow[y*m.W+x] = true
}

// Clear makes (x, y) transparent.
func (m *Indexed) Clear(x, y int) {
	if m.In(x, y) {
		m.Pix[y*m.W+x] = Transparent
		if m.Glow != nil {
			m.Glow[y*m.W+x] = false
		}
	}
}

// Rect fills a rectangle.
func (m *Indexed) Rect(x, y, w, h int, c color.RGBA) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			m.Set(i, j, c)
		}
	}
}

// Clone returns a copy of m.
func (m *Indexed) Clone() *Indexed {
	c := &Indexed{W: m.W, H: m.H, Pix: append([]uint8(nil), m.Pix...)}
	if m.Glow != nil {
		c.Glow = append([]bool(nil), m.Glow...)
	}
	return c
}

// RGBA converts m to a normal image, for drawing in the UI.
func (m *Indexed) RGBA() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, m.W, m.H))
	for y := 0; y < m.H; y++ {
		for x := 0; x < m.W; x++ {
			if i := m.At(x, y); i != Transparent {
				img.SetRGBA(x, y, pal.All[i])
			}
		}
	}
	return img
}
