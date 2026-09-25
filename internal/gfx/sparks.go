package gfx

import (
	"image/color"
	"math"
	"math/rand/v2"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/pal"
)

// Sparks are short-lived bits of light: sparks off a blow, coins from a
// chest, a shower when the hero levels up. They are drawn as art pixels.
// A nil *Sparks does nothing.
type Sparks struct {
	ps  []spark
	rng *rand.Rand
}

type spark struct {
	x, y, vx, vy float64
	fall         float64 // gravity, in pixels per tick per tick
	life, max    int
	col          color.RGBA
	size         int
}

// Burst describes a burst of sparks.
type Burst struct {
	N      int     // how many
	Speed  float64 // top speed, in screen pixels per tick
	Fall   float64 // gravity; negative rises
	Life   int     // ticks, about
	Size   int     // in screen pixels; 0 means one art pixel
	Up     bool    // thrown upwards, like a fountain, instead of every way
	Colors []color.RGBA
}

// NewSparks makes an empty set of sparks.
func NewSparks(seed uint64) *Sparks {
	return &Sparks{rng: rand.New(rand.NewPCG(seed, seed^0x5bd1e995))}
}

// Burst throws sparks out from (x, y) in screen pixels.
func (s *Sparks) Burst(x, y float64, b Burst) {
	if s == nil {
		return
	}
	if len(b.Colors) == 0 {
		b.Colors = []color.RGBA{pal.White}
	}
	size := b.Size
	if size <= 0 {
		size = ArtScale
	}
	for i := 0; i < b.N; i++ {
		a := s.rng.Float64() * 2 * math.Pi
		if b.Up {
			a = -math.Pi/2 + (s.rng.Float64()-0.5)*math.Pi*0.8
		}
		v := b.Speed * (0.35 + 0.65*s.rng.Float64())
		s.ps = append(s.ps, spark{
			x: x, y: y, vx: math.Cos(a) * v, vy: math.Sin(a) * v,
			fall: b.Fall, max: b.Life/2 + s.rng.IntN(b.Life/2+1),
			col: b.Colors[s.rng.IntN(len(b.Colors))], size: size,
		})
	}
}

// Active reports whether any sparks are still flying.
func (s *Sparks) Active() bool { return s != nil && len(s.ps) > 0 }

// Update moves the sparks on by a tick.
func (s *Sparks) Update() {
	if s == nil {
		return
	}
	live := s.ps[:0]
	for _, p := range s.ps {
		p.life++
		p.x += p.vx
		p.y += p.vy
		p.vy += p.fall
		p.vx *= 0.96
		if p.life < p.max {
			live = append(live, p)
		}
	}
	s.ps = live
}

// Draw draws the sparks, snapped to the art grid. They fade as they die.
func (s *Sparks) Draw(dst *ebiten.Image) {
	if s == nil {
		return
	}
	for _, p := range s.ps {
		k := 1 - float64(p.life)/float64(p.max)
		if k < 0.35 && p.life%2 == 1 {
			continue // flicker out
		}
		x := int(p.x) / ArtScale * ArtScale
		y := int(p.y) / ArtScale * ArtScale
		FillRect(dst, x, y, p.size, p.size, pal.Fade(p.col, min(1, 0.4+k)))
	}
}
