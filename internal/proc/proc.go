// Package proc generates pixel art procedurally. It has no Ebitengine
// dependency: generators return *image.RGBA, which the gfx package uploads.
package proc

import (
	"image"
	"image/color"
	"math"
	"math/rand/v2"
)

// NewRand returns a deterministic random source for seed.
func NewRand(seed uint64) *rand.Rand {
	return rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
}

// hash2 returns a stable pseudo-random value in [0,1) for a lattice point.
func hash2(x, y int, seed uint64) float64 {
	h := uint64(x)*0x9E3779B185EBCA87 ^ uint64(y)*0xC2B2AE3D27D4EB4F ^ seed*0x165667B19E3779F9
	h ^= h >> 33
	h *= 0xff51afd7ed558ccd
	h ^= h >> 33
	return float64(h>>11) / float64(1<<53)
}

// ValueNoise returns smooth noise in [0,1) at (x, y); scale is the lattice size
// in pixels.
func ValueNoise(x, y, scale float64, seed uint64) float64 {
	fx, fy := x/scale, y/scale
	x0, y0 := int(math.Floor(fx)), int(math.Floor(fy))
	tx, ty := smooth(fx-float64(x0)), smooth(fy-float64(y0))
	a := hash2(x0, y0, seed)
	b := hash2(x0+1, y0, seed)
	c := hash2(x0, y0+1, seed)
	d := hash2(x0+1, y0+1, seed)
	return lerp(lerp(a, b, tx), lerp(c, d, tx), ty)
}

func smooth(t float64) float64 { return t * t * (3 - 2*t) }

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

// bayer4 is a 4×4 ordered-dither matrix, normalised to [0,1).
var bayer4 = [4][4]float64{
	{0 / 16.0, 8 / 16.0, 2 / 16.0, 10 / 16.0},
	{12 / 16.0, 4 / 16.0, 14 / 16.0, 6 / 16.0},
	{3 / 16.0, 11 / 16.0, 1 / 16.0, 9 / 16.0},
	{15 / 16.0, 7 / 16.0, 13 / 16.0, 5 / 16.0},
}

// Dither picks a colour from ramp (dark to light) for brightness v in [0,1]
// using ordered dithering, which gives the classic 8-bit shading look.
func Dither(ramp []color.RGBA, v float64, x, y int) color.RGBA {
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	f := v * float64(len(ramp)-1)
	i := int(f)
	if i >= len(ramp)-1 {
		return ramp[len(ramp)-1]
	}
	if f-float64(i) > bayer4[y&3][x&3] {
		return ramp[i+1]
	}
	return ramp[i]
}

// BrickWall draws a dungeon wall of staggered bricks. ramp runs from mortar
// (darkest) to highlight (lightest).
func BrickWall(w, h int, ramp []color.RGBA, seed uint64) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	const bw, bh = 16, 8
	for y := 0; y < h; y++ {
		row := y / bh
		off := 0
		if row%2 == 1 {
			off = bw / 2
		}
		for x := 0; x < w; x++ {
			bx := (x + off) / bw
			lx, ly := (x+off)%bw, y%bh
			if lx == 0 || ly == 0 {
				img.SetRGBA(x, y, ramp[0]) // mortar
				continue
			}
			// Per-brick tone, surface noise, and a bevel: lit top-left, dark bottom-right.
			v := 0.35 + 0.25*hash2(bx, row, seed)
			v += 0.25 * (ValueNoise(float64(x), float64(y), 3, seed+1) - 0.5)
			switch {
			case ly == 1 || lx == 1:
				v += 0.2
			case ly == bh-1 || lx == bw-1:
				v -= 0.2
			}
			if hash2(x, y, seed+2) < 0.03 {
				v -= 0.3 // pits and cracks
			}
			img.SetRGBA(x, y, Dither(ramp[1:], v, x, y))
		}
	}
	return img
}

// Vignette darkens img towards its edges, as if lit by a central light.
func Vignette(img *image.RGBA, strength float64) {
	b := img.Bounds()
	cx, cy := float64(b.Dx())/2, float64(b.Dy())/2
	maxd := math.Hypot(cx, cy)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			d := math.Hypot(float64(x)-cx, float64(y)-cy) / maxd
			k := 1 - strength*d*d
			// Quantise the darkening with the dither matrix to keep it pixel-y.
			k = math.Floor(k*6+bayer4[y&3][x&3]) / 6
			if k < 0 {
				k = 0
			}
			c := img.RGBAAt(x, y)
			img.SetRGBA(x, y, color.RGBA{uint8(float64(c.R) * k), uint8(float64(c.G) * k), uint8(float64(c.B) * k), c.A})
		}
	}
}
