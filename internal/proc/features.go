package proc

import (
	"image/color"

	"github.com/halpworld/halpwords/internal/pal"
)

// Crown puts a gold crown on a boss sprite. The result is taller than the
// sprite, with the sprite at the bottom, so nothing is cut off.
func Crown(m *Indexed) *Indexed {
	const extra = 8
	out := NewIndexed(m.W, m.H+extra)
	top := m.H
	for y := 0; y < m.H; y++ {
		for x := 0; x < m.W; x++ {
			i := y*m.W + x
			if m.Pix[i] == Transparent {
				continue
			}
			top = min(top, y)
			if m.Glow != nil && m.Glow[i] {
				out.SetGlow(x, y+extra, pal.All[m.Pix[i]])
			} else {
				out.Pix[(y+extra)*m.W+x] = m.Pix[i]
			}
		}
	}
	// The band sits on the top of the head, with three points above it.
	cx, base := m.W/2, top+extra
	out.Rect(cx-6, base-3, 12, 3, pal.Yellow)
	out.Rect(cx-6, base-1, 12, 1, pal.Bronze)
	for _, px := range []int{cx - 6, cx - 1, cx + 4} {
		out.Rect(px, base-6, 2, 3, pal.Yellow)
		out.Set(px, base-7, pal.Yellow)
	}
	out.SetGlow(cx-4, base-2, pal.Rose)
	out.SetGlow(cx, base-2, pal.Cyan)
	out.SetGlow(cx+3, base-2, pal.Lime)
	outline(out)
	return out
}

// ShrineSprite draws a Save Shrine: a stone pedestal with a floating
// crystal. frame (0 or 1) moves the sparkle.
func ShrineSprite(frame int) *Indexed {
	var c canvas
	f := float64(frame & 1)
	c.rect(7, 20, 18, 3, partBody) // top slab
	c.rect(9, 23, 14, 7, partBody)
	c.rect(6, 29, 20, 3, partBody) // foot
	c.rect(15, 24, 2, 4, partAccent)
	c.rect(12, 25, 8, 1, partAccent) // a rune
	c.tri(16, 3-f, 10.5, 10-f, 21.5, 10-f, partGlow)
	c.tri(10.5, 10-f, 21.5, 10-f, 16, 18-f, partGlow)
	m := shadeMonster(&c, monsterLook{
		ramp:   []color.RGBA{pal.Night, pal.Slate, pal.Granite, pal.Stone, pal.Ash},
		accent: [2]color.RGBA{pal.Navy, pal.Cyan},
		glow:   pal.Cyan,
	})
	m.SetGlow(14, 7-frame, pal.White)
	m.SetGlow(13, 8-frame, pal.Ice)
	sx := 22 + 3*frame
	m.SetGlow(sx, 4+2*frame, pal.White)
	m.SetGlow(8-frame, 13, pal.Ice)
	return m
}

// CampfireSprite draws a campfire, flickering over frames 0 to 3, or burnt
// down to embers once used.
func CampfireSprite(frame int, used bool) *Indexed {
	var c canvas
	for i, x := range []float64{6, 11, 16, 21, 26} {
		c.disc(x, 29.5+float64(i%2)*0.5, 2.5, partBody) // ring of stones
	}
	c.line(7, 27, 25, 23, 3, partAccent) // logs
	c.line(7, 23, 25, 27, 3, partAccent)
	m := shadeMonster(&c, monsterLook{
		ramp:   []color.RGBA{pal.Night, pal.Slate, pal.Granite, pal.Stone},
		accent: [2]color.RGBA{pal.Mahogany, pal.Brown},
		glow:   pal.Orange,
	})
	rng := NewRand(uint64(frame) + 1)
	if used {
		for i := 0; i < 7; i++ {
			col := pal.Red
			if i%3 == 0 {
				col = pal.Orange
			}
			m.SetGlow(9+rng.IntN(14), 22+rng.IntN(5), col)
		}
		return m
	}
	// Flames: an outer orange tongue, a yellow core, and a white heart.
	sway := float64(frame%4) - 1.5
	for y := 4; y < 25; y++ {
		k := float64(y-4) / 20 // 0 at the tip, 1 at the base
		cx := 16 + sway*(1-k)
		w := 7*k + rng.Float64()*1.5
		for x := int(cx - w); x <= int(cx+w); x++ {
			d := (float64(x) - cx) / (w + 0.01)
			col := pal.Orange
			switch {
			case d*d < 0.15 && k > 0.5:
				col = pal.White
			case d*d < 0.4 && k > 0.25:
				col = pal.Yellow
			case d*d > 0.7 && y < 10:
				continue
			}
			m.SetGlow(x, y, col)
		}
	}
	for i := 0; i < 3; i++ { // sparks
		m.SetGlow(10+rng.IntN(12), rng.IntN(5), pal.Yellow)
	}
	return m
}

// MerchantSprite draws the merchant: a bearded trader in a pointed hat,
// swinging a lantern. frame (0 or 1) swings it.
func MerchantSprite(frame int) *Indexed {
	var c canvas
	f := float64(frame & 1)
	c.tri(16, 11, 5, 32, 27, 32, partBody) // robe
	c.rect(7, 28, 18, 4, partBody)
	c.line(21, 18, 26+f, 23, 2, partBody) // arm
	c.line(26+f, 23, 26+f, 26, 0.8, partDark)
	c.disc(26+f, 28, 2.3, partGlow) // lantern
	c.disc(16, 11, 4.5, partAccent) // face
	c.tri(11.5, 12, 20.5, 12, 16, 22, partWhite)
	c.ellipse(16, 7, 10, 2, partBody) // hat brim
	c.tri(16, 0, 10, 7, 22, 7, partBody)
	c.set(14, 10, partPupil)
	c.set(18, 10, partPupil)
	c.rect(15, 13, 3, 1, partDark) // smile
	m := shadeMonster(&c, monsterLook{
		ramp:   []color.RGBA{pal.Night, pal.Indigo, pal.Blue, pal.Sky},
		accent: [2]color.RGBA{pal.Tan, pal.Skin},
		glow:   pal.Yellow,
	})
	m.SetGlow(17, 1, pal.Yellow) // a star on the hat
	return m
}
