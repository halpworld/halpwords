package proc

import (
	"image"
	"image/color"
	"math"

	"github.com/halpworld/halpwords/internal/pal"
)

// TexSize is the width and height of dungeon wall, floor and ceiling textures.
const TexSize = 32

// Theme is the look of a dungeon floor. Each ramp runs dark to light.
type Theme struct {
	Name  string
	Wall  []color.RGBA
	Floor []color.RGBA
	Ceil  []color.RGBA
	Moss  []color.RGBA // growth on walls
}

// Themes cycle as the hero goes deeper.
var Themes = []Theme{
	{"The Crypt",
		[]color.RGBA{pal.Black, pal.Night, pal.Slate, pal.Granite, pal.Stone, pal.Ash},
		[]color.RGBA{pal.Black, pal.Night, pal.Slate, pal.Granite, pal.Stone},
		[]color.RGBA{pal.Black, pal.Night, pal.Plum, pal.Mahogany},
		[]color.RGBA{pal.Forest, pal.Olive}},
	{"Mossy Cellars",
		[]color.RGBA{pal.Black, pal.Night, pal.Olive, pal.Bronze, pal.Moss, pal.Tan},
		[]color.RGBA{pal.Black, pal.Night, pal.Slate, pal.Olive, pal.Forest},
		[]color.RGBA{pal.Black, pal.Night, pal.Slate, pal.Olive},
		[]color.RGBA{pal.Forest, pal.Green}},
	{"Flooded Caves",
		[]color.RGBA{pal.Black, pal.Night, pal.Indigo, pal.Navy, pal.Teal, pal.Cyan},
		[]color.RGBA{pal.Black, pal.Night, pal.Indigo, pal.Navy, pal.Teal},
		[]color.RGBA{pal.Black, pal.Night, pal.Indigo, pal.Navy},
		[]color.RGBA{pal.Teal, pal.Green}},
	{"Ice Halls",
		[]color.RGBA{pal.Night, pal.Indigo, pal.Navy, pal.Steel, pal.Ice, pal.White},
		[]color.RGBA{pal.Night, pal.Indigo, pal.Navy, pal.Steel, pal.Ice},
		[]color.RGBA{pal.Black, pal.Night, pal.Indigo, pal.Navy},
		[]color.RGBA{pal.Sky, pal.Cyan}},
	{"Lava Forge",
		[]color.RGBA{pal.Black, pal.Plum, pal.Mahogany, pal.Brown, pal.Red, pal.Orange},
		[]color.RGBA{pal.Black, pal.Night, pal.Plum, pal.Mahogany, pal.Brown},
		[]color.RGBA{pal.Black, pal.Night, pal.Plum, pal.Mahogany},
		[]color.RGBA{pal.Orange, pal.Yellow}},
	{"Amethyst Vaults",
		[]color.RGBA{pal.Black, pal.Night, pal.Plum, pal.Purple, pal.Pink, pal.Skin},
		[]color.RGBA{pal.Black, pal.Night, pal.Indigo, pal.Plum, pal.Purple},
		[]color.RGBA{pal.Black, pal.Night, pal.Indigo, pal.Plum},
		[]color.RGBA{pal.Pink, pal.Skin}},
}

// ThemeFor returns the theme for a floor depth; each lasts two floors.
func ThemeFor(depth int) *Theme {
	return &Themes[((depth-1)/2)%len(Themes)]
}

// ToIndexed converts a palette-coloured image.
func ToIndexed(img *image.RGBA) *Indexed {
	b := img.Bounds()
	m := NewIndexed(b.Dx(), b.Dy())
	for y := 0; y < m.H; y++ {
		for x := 0; x < m.W; x++ {
			c := img.RGBAAt(b.Min.X+x, b.Min.Y+y)
			if c.A != 0 {
				m.Set(x, y, c)
			}
		}
	}
	return m
}

// WallTexture draws a brick wall. Variant 1 adds moss, variant 2 a crack.
func WallTexture(t *Theme, seed uint64, variant int) *Indexed {
	m := ToIndexed(BrickWall(TexSize, TexSize, t.Wall, seed))
	switch variant {
	case 1:
		for y := TexSize / 2; y < TexSize; y++ {
			for x := 0; x < TexSize; x++ {
				v := ValueNoise(float64(x), float64(y), 4, seed+9) + float64(y-TexSize/2)/TexSize
				if v > 0.95 {
					m.Set(x, y, t.Moss[int(hash2(x, y, seed)*2)])
				}
			}
		}
	case 2:
		rng := NewRand(seed)
		x, y := 8+rng.IntN(16), 2
		for y < TexSize-4 {
			m.Set(x, y, t.Wall[0])
			m.Set(x+1, y, t.Wall[1])
			y++
			x += rng.IntN(3) - 1
		}
	}
	return m
}

// TorchWall draws a wall with a burning torch. frame animates the flame.
func TorchWall(t *Theme, seed uint64, frame int) *Indexed {
	m := WallTexture(t, seed, 0)
	// Iron bracket and wooden handle.
	m.Rect(13, 20, 6, 2, pal.Night)
	m.Rect(14, 22, 4, 1, pal.Granite)
	m.Rect(15, 12, 2, 10, pal.Brown)
	m.Set(15, 12, pal.Tan)
	m.Rect(14, 11, 4, 2, pal.Mahogany)
	// Flame: a teardrop that flickers between frames.
	rng := NewRand(uint64(frame) + 77)
	for y := 0; y < 10; y++ {
		fy := float64(y) / 9 // 0 at the tip, 1 at the base
		half := math.Sin(fy*math.Pi*0.8) * 3.2
		sway := math.Sin(float64(frame)*2.1+fy*3) * (1 - fy) * 1.2
		for x := -4; x <= 4; x++ {
			d := math.Abs(float64(x) - sway)
			if d > half+rng.Float64()*0.6 {
				continue
			}
			heat := (1 - d/(half+0.5)) * (0.4 + 0.6*fy)
			c := pal.Red
			switch {
			case heat > 0.55:
				c = pal.Yellow
			case heat > 0.3:
				c = pal.Orange
			}
			if y == 0 && x != int(math.Round(sway)) {
				continue
			}
			m.SetGlow(16+x, 1+y, c)
		}
	}
	m.SetGlow(16, 8, pal.White)
	return m
}

// DoorTexture draws a wooden door in a stone frame. A sealed door carries a
// glowing rune.
func DoorTexture(t *Theme, seed uint64, sealed bool) *Indexed {
	m := WallTexture(t, seed, 0)
	const x0, y0, x1 = 5, 4, 27
	// Stone frame.
	for y := y0 - 2; y < TexSize; y++ {
		for x := x0 - 2; x < x1+2; x++ {
			if arch(x, y, x0-2, y0-2, x1+2) {
				m.Set(x, y, t.Wall[len(t.Wall)-2])
			}
		}
	}
	for y := y0; y < TexSize; y++ {
		for x := x0; x < x1; x++ {
			if !arch(x, y, x0, y0, x1) {
				continue
			}
			// Vertical planks with grain.
			lx := (x - x0) % 6
			c := pal.Brown
			switch {
			case lx == 0:
				c = pal.Plum
			case lx == 1:
				c = pal.Tan
			case hash2(x, y/3, seed) < 0.25:
				c = pal.Mahogany
			}
			m.Set(x, y, c)
		}
	}
	// Iron bands with rivets.
	for _, by := range []int{11, 24} {
		for x := x0; x < x1; x++ {
			m.Set(x, by, pal.Granite)
			m.Set(x, by+1, pal.Night)
			if (x-x0)%5 == 2 {
				m.Set(x, by, pal.Ash)
			}
		}
	}
	if sealed {
		// A ring of runes that glows in the dark.
		cx, cy := 16.0, 18.0
		for y := 10; y < 27; y++ {
			for x := 8; x < 25; x++ {
				d := math.Hypot(float64(x)-cx+0.5, float64(y)-cy+0.5)
				if d > 5.5 && d < 7.2 {
					c := pal.Sky
					if hash2(x, y, 3) < 0.35 {
						c = pal.Cyan
					}
					m.SetGlow(x, y, c)
				}
			}
		}
		for _, p := range [][2]int{{16, 14}, {16, 15}, {16, 16}, {16, 17}, {16, 18}, {16, 19}, {16, 20}, {16, 21}, {14, 16}, {15, 17}, {17, 17}, {18, 16}, {14, 20}, {18, 20}} {
			m.SetGlow(p[0], p[1], pal.Ice)
		}
	} else {
		m.Rect(21, 17, 2, 3, pal.Bronze)
		m.Set(21, 17, pal.Yellow)
	}
	return m
}

// arch reports whether (x, y) is inside a round-topped doorway.
func arch(x, y, x0, y0, x1 int) bool {
	if x < x0 || x >= x1 || y < y0 {
		return false
	}
	r := float64(x1-x0) / 2
	cy := float64(y0) + r*0.7
	if float64(y) >= cy {
		return true
	}
	cx := float64(x0) + r
	dx := (float64(x) + 0.5 - cx) / r
	dy := (float64(y) + 0.5 - cy) / (r * 0.7)
	return dx*dx+dy*dy <= 1
}

// FloorTexture draws flagstones.
func FloorTexture(t *Theme, seed uint64) *Indexed {
	m := NewIndexed(TexSize, TexSize)
	ramp := t.Floor
	// Four stones per tile with a ragged seam.
	seamX := 16 + int(hash2(1, 0, seed)*6) - 3
	for y := 0; y < TexSize; y++ {
		for x := 0; x < TexSize; x++ {
			top := y < 16
			sx := seamX
			if !top {
				sx = (seamX + 11) % TexSize
			}
			edge := y == 0 || y == 16 || x == sx || (x == 0 && sx != 0)
			if edge {
				m.Set(x, y, ramp[0])
				continue
			}
			stone := 0
			if x > sx {
				stone = 1
			}
			if !top {
				stone += 2
			}
			v := 0.35 + 0.3*hash2(stone, 0, seed)
			v += 0.3 * (ValueNoise(float64(x), float64(y), 4, seed+3) - 0.5)
			if y == 1 || y == 17 || x == sx+1 {
				v += 0.15
			}
			if hash2(x, y, seed+5) < 0.04 {
				v -= 0.35
			}
			m.Set(x, y, Dither(ramp[1:], v, x, y))
		}
	}
	return m
}

// CeilingTexture draws dark stone blocks with a timber beam.
func CeilingTexture(t *Theme, seed uint64) *Indexed {
	m := NewIndexed(TexSize, TexSize)
	ramp := t.Ceil
	for y := 0; y < TexSize; y++ {
		for x := 0; x < TexSize; x++ {
			if y >= 13 && y < 19 {
				// Beam.
				c := pal.Mahogany
				switch {
				case y == 13:
					c = pal.Brown
				case y == 18:
					c = pal.Black
				case hash2(x/4, y, seed) < 0.3:
					c = pal.Plum
				}
				m.Set(x, y, c)
				continue
			}
			if x%16 == 0 || y == 0 {
				m.Set(x, y, ramp[0])
				continue
			}
			v := 0.4 + 0.3*(ValueNoise(float64(x), float64(y), 5, seed)-0.5)
			m.Set(x, y, Dither(ramp[1:], v, x, y))
		}
	}
	return m
}

// StairsTexture draws stairs going down into the dark, seen from above.
func StairsTexture(t *Theme, seed uint64) *Indexed {
	m := FloorTexture(t, seed)
	ramp := t.Wall
	for step := 0; step < 6; step++ {
		in := 3 + step*2
		c := ramp[max(0, len(ramp)-2-step)]
		for y := in; y < TexSize-3; y++ {
			for x := in; x < TexSize-in; x++ {
				m.Set(x, y, c)
			}
		}
		// A lit front edge on each step.
		for x := in; x < TexSize-in; x++ {
			m.Set(x, in, ramp[min(len(ramp)-1, len(ramp)-1-step)])
		}
	}
	m.Rect(15, TexSize-6, 2, 3, pal.Black)
	return m
}

// ChestSprite draws a treasure chest, closed or open.
func ChestSprite(open bool) *Indexed {
	const w, h = 26, 20
	m := NewIndexed(w, h)
	// Box.
	for y := 9; y < h; y++ {
		for x := 1; x < w-1; x++ {
			c := pal.Brown
			switch {
			case y == h-1:
				c = pal.Plum
			case (x-1)%6 == 0:
				c = pal.Mahogany
			case y == 9:
				c = pal.Tan
			}
			m.Set(x, y, c)
		}
	}
	for _, x := range []int{3, w - 5} {
		m.Rect(x, 9, 2, h-9, pal.Bronze)
	}
	if open {
		// Lid tipped back, with treasure inside.
		m.Rect(2, 1, w-4, 6, pal.Mahogany)
		m.Rect(3, 2, w-6, 5, pal.Night)
		for x := 3; x < w-3; x++ {
			for y := 6; y < 10; y++ {
				if hash2(x, y, 11) < 0.7 {
					c := pal.Yellow
					if hash2(x, y, 12) < 0.4 {
						c = pal.Orange
					}
					m.SetGlow(x, y, c)
				}
			}
		}
		m.SetGlow(8, 5, pal.White)
		m.SetGlow(17, 6, pal.White)
	} else {
		// Rounded lid and lock.
		for y := 2; y < 9; y++ {
			inset := 0
			if y < 4 {
				inset = 4 - y
			}
			for x := 1 + inset; x < w-1-inset; x++ {
				c := pal.Brown
				switch {
				case y == 2:
					c = pal.Tan
				case y == 8:
					c = pal.Plum
				case (x-1)%6 == 0:
					c = pal.Mahogany
				}
				m.Set(x, y, c)
			}
		}
		for _, x := range []int{3, w - 5} {
			m.Rect(x, 2, 2, 7, pal.Bronze)
		}
		m.Rect(w/2-2, 7, 4, 5, pal.Bronze)
		m.Set(w/2-1, 8, pal.Yellow)
		m.Set(w/2, 9, pal.Black)
	}
	outline(m)
	return m
}

// outline surrounds the opaque pixels of m with black.
func outline(m *Indexed) {
	var edge []int
	for y := 0; y < m.H; y++ {
		for x := 0; x < m.W; x++ {
			if m.At(x, y) != Transparent {
				continue
			}
			if m.At(x-1, y) != Transparent || m.At(x+1, y) != Transparent || m.At(x, y-1) != Transparent || m.At(x, y+1) != Transparent {
				edge = append(edge, y*m.W+x)
			}
		}
	}
	for _, i := range edge {
		m.Pix[i] = 0 // pal.Black
	}
}
