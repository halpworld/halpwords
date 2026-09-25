package gfx

import (
	"image"
	"image/color"
	"math"
	"math/rand/v2"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/pkg/proc"
)

type particle struct {
	x, y, vx, vy float64
	life, max    int
}

// Torch is a wall torch: a bracket, a particle flame, and a flickering glow.
type Torch struct {
	X, Y  int // screen position of the flame base
	ps    []particle
	rng   *rand.Rand
	glow  *ebiten.Image
	flick float64
}

var flameRamp = []color.RGBA{pal.Plum, pal.Red, pal.Orange, pal.Yellow, pal.White}

// NewTorch creates a torch whose flame base is at screen (x, y).
func NewTorch(x, y int, seed uint64) *Torch {
	return &Torch{X: x, Y: y, rng: proc.NewRand(seed), glow: glowImage(56)}
}

// Update advances the flame by one tick.
func (t *Torch) Update() {
	for i := 0; i < 4; i++ {
		t.ps = append(t.ps, particle{
			x:    float64(t.X) + (t.rng.Float64()-0.5)*10,
			y:    float64(t.Y),
			vx:   (t.rng.Float64() - 0.5) * 0.4,
			vy:   -0.6 - t.rng.Float64()*0.9,
			max:  14 + t.rng.IntN(14),
			life: 0,
		})
	}
	live := t.ps[:0]
	for _, p := range t.ps {
		p.life++
		p.x += p.vx + math.Sin(float64(p.life)*0.4)*0.2
		p.y += p.vy
		if p.life < p.max {
			live = append(live, p)
		}
	}
	t.ps = live
	t.flick = 0.8 + 0.2*math.Sin(float64(t.rng.IntN(1000))) // jittery light
}

// Draw draws the glow, bracket and flame.
func (t *Torch) Draw(dst *ebiten.Image) {
	op := &ebiten.DrawImageOptions{}
	gw := t.glow.Bounds().Dx() * ArtScale
	op.GeoM.Scale(ArtScale, ArtScale)
	op.GeoM.Translate(float64(t.X-gw/2), float64(t.Y-gw/2-8))
	op.ColorScale.ScaleAlpha(float32(t.flick))
	op.Blend = ebiten.BlendLighter
	dst.DrawImage(t.glow, op)

	// Bracket.
	FillRect(dst, t.X-6, t.Y, 12, 4, pal.Granite)
	FillRect(dst, t.X-2, t.Y+4, 4, 14, pal.Mahogany)
	FillRect(dst, t.X-4, t.Y+16, 8, 4, pal.Granite)

	// Hot core at the base of the flame.
	FillRect(dst, t.X-6, t.Y-6, 12, 6, pal.Orange)
	FillRect(dst, t.X-4, t.Y-8, 8, 4, pal.Yellow)

	for _, p := range t.ps {
		k := 1 - float64(p.life)/float64(p.max)
		c := flameRamp[int(k*float64(len(flameRamp)-1)+0.5)]
		size := ArtScale * 2
		if k > 0.6 {
			size = ArtScale * 3
		}
		// Snap to the art grid so the flame looks pixelated.
		x := int(p.x) / ArtScale * ArtScale
		y := int(p.y) / ArtScale * ArtScale
		FillRect(dst, x, y, size, size, c)
	}
}

// glowImage is a dithered warm disc, drawn additively around light sources.
func glowImage(r int) *ebiten.Image {
	img := image.NewRGBA(image.Rect(0, 0, r*2, r*2))
	warm := pal.Fade(pal.Orange, 0.3)
	for y := 0; y < r*2; y++ {
		for x := 0; x < r*2; x++ {
			d := math.Hypot(float64(x-r)+0.5, float64(y-r)+0.5) / float64(r)
			if d >= 1 {
				continue
			}
			v := (1 - d) * (1 - d)
			if proc.Dither([]color.RGBA{{}, warm}, v*1.6, x, y) == warm {
				img.SetRGBA(x, y, warm)
			}
		}
	}
	return ebiten.NewImageFromImage(img)
}
