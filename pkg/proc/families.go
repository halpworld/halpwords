package proc

import (
	"image/color"
	"math"

	"github.com/halpworld/halpwords/internal/pal"
)

// A world's walls, floors and ceilings are built from surface families:
// one painter per material, each filling a tiling TexSize×TexSize texture.
// Modifiers then draw details on top, such as moss, cracks or a gear, and
// the palette ramps in a Look colour the result. A new world is a new mix of
// these, not new code.

// Look is the palette a surface paints with. Each ramp runs dark to light.
type Look struct {
	Base   []color.RGBA // the material; Base[0] is the darkest (mortar, gaps)
	Accent []color.RGBA // details: moss, book spines, flowers, glyphs, brass
}

// painter paints a whole surface family. Painters must tile seamlessly.
type painter func(m *Indexed, l Look, seed uint64)

// modifier draws something over a painted surface.
type modifier func(m *Indexed, l Look, seed uint64)

// animator redraws part of a painted surface for animation frame 0 to 3,
// such as ripples on water. Frame 0 is the still look.
type animator func(m *Indexed, l Look, seed uint64, frame int)

// Frames is the number of animation frames of an animated surface.
const Frames = 4

// Surface is a family, its palette, and modifiers applied in order.
type Surface struct {
	Paint painter
	Look  Look
	Mods  []modifier
	Anim  animator // nil for a still surface
}

// Make paints the surface's first frame.
func (s Surface) Make(seed uint64) *Indexed { return s.Frame(seed, 0) }

// Frame paints animation frame f of the surface.
func (s Surface) Frame(seed uint64, f int) *Indexed {
	m := NewIndexed(TexSize, TexSize)
	s.Paint(m, s.Look, seed)
	for i, mod := range s.Mods {
		mod(m, s.Look, seed+uint64(i)*101)
	}
	if s.Anim != nil {
		s.Anim(m, s.Look, seed, f)
	}
	return m
}

// tileNoise is value noise that wraps every TexSize pixels; cells is the
// number of lattice cells across the tile.
func tileNoise(x, y float64, cells int, seed uint64) float64 {
	return wrapNoise(x, y, TexSize, cells, seed)
}

// wrapNoise is value noise that wraps every size pixels in x and y.
func wrapNoise(x, y float64, size, cells int, seed uint64) float64 {
	s := float64(size) / float64(cells)
	fx, fy := x/s, y/s
	x0, y0 := int(math.Floor(fx)), int(math.Floor(fy))
	tx, ty := smooth(fx-float64(x0)), smooth(fy-float64(y0))
	x0, y0 = wrap(x0, cells), wrap(y0, cells)
	x1, y1 := wrap(x0+1, cells), wrap(y0+1, cells)
	a, b := hash2(x0, y0, seed), hash2(x1, y0, seed)
	c, d := hash2(x0, y1, seed), hash2(x1, y1, seed)
	return lerp(lerp(a, b, tx), lerp(c, d, tx), ty)
}

func wrap(v, n int) int { return ((v % n) + n) % n }

// cellPoints scatters n points over the tile for cellular patterns.
func cellPoints(n int, seed uint64) [][2]float64 {
	rng := NewRand(seed)
	pts := make([][2]float64, n)
	for i := range pts {
		pts[i] = [2]float64{rng.Float64() * TexSize, rng.Float64() * TexSize}
	}
	return pts
}

// cell is where a pixel sits in a cellular pattern.
type cell struct {
	d1, d2 float64 // distances to the nearest and second nearest point
	id     int     // the nearest point
	dx, dy float64 // offset from the nearest point
}

// nearest finds the cell of (x, y) among pts, wrapping around the tile.
func nearest(x, y float64, pts [][2]float64) cell {
	c := cell{d1: math.Inf(1), d2: math.Inf(1)}
	for i, p := range pts {
		ox := math.Mod(x-p[0]+1.5*TexSize, TexSize) - TexSize/2
		oy := math.Mod(y-p[1]+1.5*TexSize, TexSize) - TexSize/2
		d := hypot(ox, oy)
		switch {
		case d < c.d1:
			c.d2 = c.d1
			c.d1, c.id, c.dx, c.dy = d, i, ox, oy
		case d < c.d2:
			c.d2 = d
		}
	}
	return c
}

// gap is how far a pixel is from the edge between two cells.
func (c cell) gap() float64 { return c.d2 - c.d1 }

func last(r []color.RGBA) color.RGBA { return r[len(r)-1] }

// pick returns the colour at fraction f along ramp r.
func pick(r []color.RGBA, f float64) color.RGBA {
	return r[min(len(r)-1, max(0, int(f*float64(len(r)))))]
}

// Wall families.

// wallBricks is the classic staggered brick wall of The Crypt.
func wallBricks(m *Indexed, l Look, seed uint64) {
	copy(m.Pix, ToIndexed(BrickWall(TexSize, TexSize, l.Base, seed)).Pix)
}

// wallCave is rough rock: rounded lumps with dark crevices.
func wallCave(m *Indexed, l Look, seed uint64) {
	pts := cellPoints(8, seed)
	for y := range TexSize {
		for x := range TexSize {
			c := nearest(float64(x)+0.5, float64(y)+0.5, pts)
			if c.gap() < 1.1 {
				m.Set(x, y, l.Base[0])
				continue
			}
			r := (c.d1 + c.d2) / 2
			v := 0.42 + 0.22*(hash2(c.id, 0, seed)-0.5)
			v -= 0.45 * (c.dx + c.dy) / (r + 1) // lit from the top left
			if c.gap() < 2.5 {
				v -= 0.15
			}
			v += 0.25 * (tileNoise(float64(x), float64(y), 8, seed+3) - 0.5)
			m.Set(x, y, Dither(l.Base[1:], v, x, y))
		}
	}
}

// wallIce is big ice blocks with bright edges and diagonal glints.
func wallIce(m *Indexed, l Look, seed uint64) {
	rows := [...]int{0, 11, 22, TexSize}
	for r := range len(rows) - 1 {
		y0, bh := rows[r], rows[r+1]-rows[r]
		off := (r % 2) * 9
		for ly := range bh {
			y := y0 + ly
			for x := range TexSize {
				lx, b := (x+off)%16, (x+off)/16
				switch {
				case lx == 15 || ly == bh-1:
					m.Set(x, y, l.Base[0])
					continue
				case lx == 0 || ly == 0:
					m.Set(x, y, last(l.Base))
					continue
				}
				v := 0.15 + 0.35*float64(bh-ly)/float64(bh) + 0.2*(hash2(b, r, seed)-0.5)
				if d := (lx - ly + int(hash2(b, r, seed+1)*10) + 18) % 9; d <= 1 {
					v += 0.5
				}
				if ly == bh-2 || lx == 14 {
					v -= 0.2
				}
				v += 0.12 * (tileNoise(float64(x), float64(y), 8, seed+2) - 0.5)
				m.Set(x, y, Dither(l.Base[1:], v, x, y))
			}
		}
	}
}

// wallBasalt is dark faceted stone columns with cross cracks.
func wallBasalt(m *Indexed, l Look, seed uint64) {
	edges := [...]int{0, 7, 15, 21, 27, TexSize}
	for c := range len(edges) - 1 {
		x0, w := edges[c], edges[c+1]-edges[c]
		crack := int(hash2(c, 0, seed) * TexSize)
		for lx := range w {
			x := x0 + lx
			for y := range TexSize {
				if lx == 0 || y == crack || (y == (crack+16)%TexSize && c%2 == 0) {
					m.Set(x, y, l.Base[0])
					continue
				}
				// Two faces meeting at a ridge.
				v := 0.55 - 0.5*math.Abs(float64(lx)/float64(w)-0.35)
				if lx == 1 {
					v += 0.25
				}
				if lx == w-1 || y == (crack+1)%TexSize {
					v -= 0.2
				}
				v += 0.2 * (tileNoise(float64(x), float64(y), 16, seed+4) - 0.5)
				m.Set(x, y, Dither(l.Base[1:], v, x, y))
			}
		}
	}
}

// wallMetal is riveted, bevelled metal plates.
func wallMetal(m *Indexed, l Look, seed uint64) {
	for y := range TexSize {
		for x := range TexSize {
			lx, ly := x%16, y%16
			corner := func(n int) bool { return (lx == n || lx == n+10) && (ly == n || ly == n+10) }
			switch {
			case lx == 0 || ly == 0 || corner(4):
				m.Set(x, y, l.Base[0])
				continue
			case corner(3):
				m.Set(x, y, last(l.Base)) // a rivet
				continue
			}
			v := 0.45 + 0.15*(hash2(x/16, y/16, seed)-0.5)
			switch {
			case lx == 1 || ly == 1:
				v += 0.3
			case lx == 15 || ly == 15:
				v -= 0.25
			}
			v += 0.1 * (hash2(x, 0, seed+1) - 0.5) // brushed
			m.Set(x, y, Dither(l.Base[1:], v, x, y))
		}
	}
}

// wallCrystal is big diamond-shaped facets with lit and shaded edges.
func wallCrystal(m *Indexed, l Look, seed uint64) {
	const s = 16
	for y := range TexSize {
		for x := range TexSize {
			u, w := x+y, x-y+4*s
			switch {
			case u%s == 0:
				m.Set(x, y, last(l.Base))
				continue
			case w%s == 0:
				m.Set(x, y, l.Base[0])
				continue
			}
			v := 0.2 + 0.6*hash2(u/s%4, w/s%4, seed)
			v += 0.3 * float64(w%s-u%s) / s
			m.Set(x, y, Dither(l.Base[1:], v, x, y))
		}
	}
}

// stoneRamp colours pebbles stuck in earth walls.
var stoneRamp = []color.RGBA{pal.Night, pal.Granite, pal.Stone, pal.Ash}

// wallEarth is packed earth with stones and roots growing down through it.
func wallEarth(m *Indexed, l Look, seed uint64) {
	stones := cellPoints(9, seed+5)
	for y := range TexSize {
		for x := range TexSize {
			c := nearest(float64(x)+0.5, float64(y)+0.5, stones)
			if r := 1.8 + 1.6*hash2(c.id, 0, seed); c.d1 < r {
				m.Set(x, y, Dither(stoneRamp, 0.5-0.5*(c.dx+c.dy)/r, x, y))
				continue
			}
			v := 0.45 + 0.3*(tileNoise(float64(x), float64(y), 4, seed)-0.5) + 0.2*(tileNoise(float64(x), float64(y), 16, seed+1)-0.5)
			m.Set(x, y, Dither(l.Base[1:3], v, x, y))
		}
	}
	rng := NewRand(seed + 2)
	root := func(x, y, dx float64, n int) {
		for i := range n {
			px, py := wrap(int(math.Floor(x)), TexSize), int(y)
			c := l.Base[3]
			if i%3 == 0 {
				c = last(l.Base)
			}
			m.Set(px, py, c)
			m.Set((px+1)%TexSize, py, l.Base[0])
			y++
			x += dx + rng.Float64()*0.8 - 0.4
		}
	}
	for i := range 2 {
		x := float64(5+i*15) + rng.Float64()*5
		root(x, 0, 0, TexSize)
		for range 2 {
			by, dx := 6+rng.Float64()*18, 0.9
			if rng.IntN(2) == 0 {
				dx = -dx
			}
			root(x, by, dx, 5+rng.IntN(5))
		}
	}
}

// glyphs are 8×8 carvings for sandstone, one bit per pixel.
var glyphs = [][8]uint8{
	{0x00, 0x3c, 0x42, 0x99, 0x42, 0x3c, 0x10, 0x18}, // eye
	{0x18, 0x24, 0x24, 0x18, 0x7e, 0x18, 0x18, 0x18}, // ankh
	{0x0c, 0x1c, 0x08, 0x3c, 0x7c, 0x14, 0x14, 0x36}, // bird
	{0x10, 0x54, 0x38, 0xfe, 0x38, 0x54, 0x10, 0x00}, // sun
	{0x00, 0x44, 0xaa, 0x11, 0x00, 0x44, 0xaa, 0x11}, // waves
	{0x18, 0x3c, 0x18, 0x7e, 0x18, 0x24, 0x42, 0x00}, // person
}

// glyphAt reports whether glyph g has a pixel at (x, y).
func glyphAt(g [8]uint8, x, y int) bool {
	return x >= 0 && y >= 0 && x < 8 && y < 8 && g[y]>>(7-x)&1 == 1
}

// wallSandstone is large sandstone blocks, each carved with a glyph. About
// half the glyphs are inlaid with glowing Accent paint.
func wallSandstone(m *Indexed, l Look, seed uint64) {
	for y := range TexSize {
		row := y / 16
		off := row * 8
		for x := range TexSize {
			lx, ly := (x+off)%16, y%16
			b := (x + off) / 16 % 2
			if lx == 0 || ly == 0 {
				m.Set(x, y, l.Base[0])
				continue
			}
			g := glyphs[int(hash2(b, row, seed+2)*float64(len(glyphs)))]
			switch gx, gy := lx-4, ly-4; {
			case glyphAt(g, gx, gy):
				if len(l.Accent) > 0 && hash2(b, row, seed+3) < 0.5 {
					m.SetGlow(x, y, pick(l.Accent, hash2(x, y, seed+4)))
				} else {
					m.Set(x, y, l.Base[0])
				}
				continue
			case glyphAt(g, gx-1, gy-1):
				m.Set(x, y, last(l.Base)) // the lit lower edge of the carving
				continue
			}
			v := 0.55 + 0.1*(hash2(b, row, seed)-0.5)
			switch {
			case lx == 1 || ly == 1:
				v += 0.2
			case lx == 15 || ly == 15:
				v -= 0.2
			}
			v += 0.15 * (tileNoise(float64(x), float64(y), 16, seed+1) - 0.5)
			m.Set(x, y, Dither(l.Base[1:], v, x, y))
		}
	}
}

// wallBooks is a bookcase with two shelves of book spines. Accent holds
// spine colours in pairs (dark, light).
func wallBooks(m *Indexed, l Look, seed uint64) {
	for y := range TexSize {
		for x := range TexSize {
			switch ly := y % 16; ly {
			case 14:
				m.Set(x, y, last(l.Base))
			case 15:
				m.Set(x, y, l.Base[2])
			default:
				m.Set(x, y, Dither(l.Base[:2], 0.3+0.4*float64(ly)/14, x, y))
			}
		}
	}
	pairs := len(l.Accent) / 2
	for shelf := range 2 {
		bottom := shelf*16 + 13
		x := 1
		for i := 0; x < TexSize-1; i++ {
			w := min(2+int(hash2(i, shelf, seed)*3), TexSize-1-x)
			if hash2(i, shelf, seed+2) < 0.08 {
				x += w // a gap on the shelf
				continue
			}
			ht := 8 + int(hash2(i, shelf, seed+1)*6)
			p := int(hash2(i, shelf, seed+3) * float64(pairs))
			book(m, x, bottom-ht+1, w, ht, l.Accent[2*p], l.Accent[2*p+1], l.Base[0], hash2(i, shelf, seed+4) < 0.5)
			x += w
		}
	}
}

// book draws one spine of a bookcase.
func book(m *Indexed, x0, y0, w, h int, dark, light, shadow color.RGBA, gilded bool) {
	for x := x0; x < x0+w; x++ {
		for y := y0; y < y0+h; y++ {
			c := dark
			switch {
			case x == x0:
				c = light
			case x == x0+w-1 && w > 2:
				c = shadow
			case gilded && (y == y0+2 || y == y0+h-3):
				c = pal.Yellow
			}
			m.Set(x, y, c)
		}
	}
}

// wallHedge is a clipped hedge of small leaves.
func wallHedge(m *Indexed, l Look, seed uint64) {
	pts := cellPoints(46, seed)
	for y := range TexSize {
		for x := range TexSize {
			c := nearest(float64(x)+0.5, float64(y)+0.5, pts)
			v := 0.25 + 0.2*(tileNoise(float64(x), float64(y), 4, seed+1)-0.5)
			if c.d1 < 2.3 {
				v = 0.5 + 0.25*(hash2(c.id, 0, seed)-0.5) - 0.2*(c.dx+c.dy)/2.3
			}
			m.Set(x, y, Dither(l.Base, v, x, y))
		}
	}
}

// Floor families.

// floorFlagstones is four big stones with ragged seams.
func floorFlagstones(m *Indexed, l Look, seed uint64) {
	ramp := l.Base
	seamX := 16 + int(hash2(1, 0, seed)*6) - 3
	for y := range TexSize {
		for x := range TexSize {
			top := y < 16
			sx := seamX
			if !top {
				sx = (seamX + 11) % TexSize
			}
			if y == 0 || y == 16 || x == sx || (x == 0 && sx != 0) {
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
}

// floorCobbles is small round stones; Accent, if any, grows in the gaps.
func floorCobbles(m *Indexed, l Look, seed uint64) {
	pts := cellPoints(16, seed)
	for y := range TexSize {
		for x := range TexSize {
			c := nearest(float64(x)+0.5, float64(y)+0.5, pts)
			if c.gap() < 1 {
				col := l.Base[0]
				if len(l.Accent) > 0 && tileNoise(float64(x), float64(y), 4, seed+7) > 0.45 {
					col = pick(l.Accent, hash2(x, y, seed))
				}
				m.Set(x, y, col)
				continue
			}
			v := 0.45 + 0.25*(hash2(c.id, 1, seed)-0.5) - 0.4*(c.dx+c.dy)/((c.d1+c.d2)/2+1)
			m.Set(x, y, Dither(l.Base[1:], v, x, y))
		}
	}
}

// floorSand is rippled sand with specks.
func floorSand(m *Indexed, l Look, seed uint64) {
	for y := range TexSize {
		for x := range TexSize {
			n := tileNoise(float64(x), float64(y), 4, seed)
			ph := 2*math.Pi*float64(x+3*y)/TexSize + 4*n
			v := 0.5 + 0.22*math.Sin(2*ph) + 0.15*(tileNoise(float64(x), float64(y), 16, seed+1)-0.5)
			if hash2(x, y, seed+2) < 0.03 {
				v -= 0.3
			}
			m.Set(x, y, Dither(l.Base, v, x, y))
		}
	}
}

// floorWater is dark, still water with a few glints. animRipples moves
// ripples across it.
func floorWater(m *Indexed, l Look, seed uint64) {
	for y := range TexSize {
		for x := range TexSize {
			v := 0.25 + 0.25*(tileNoise(float64(x), float64(y), 8, seed+1)-0.5)
			m.Set(x, y, Dither(l.Base, v, x, y))
		}
	}
	rng := NewRand(seed + 3)
	for range 3 {
		m.SetGlow(rng.IntN(TexSize), rng.IntN(TexSize), pal.White)
	}
}

// animRipples draws wavy ripple highlights (Accent) on water. Each frame
// moves them a quarter of their spacing, so four frames loop.
func animRipples(m *Indexed, l Look, seed uint64, frame int) {
	const spacing = 16
	shift := float64(frame * spacing / Frames)
	for y := range TexSize {
		for x := range TexSize {
			if tileNoise(float64(x), float64(y), 8, seed+4) <= 0.45 {
				continue // ripples come in dashes
			}
			n := tileNoise(float64(x), float64(y), 4, seed)
			wave := float64(y) - shift + 1.8*math.Sin(2*math.Pi*float64(x)/16+3*n)
			switch f := math.Mod(wave+4*spacing, spacing); {
			case f < 0.9:
				m.Set(x, y, last(l.Accent))
			case f < 1.9:
				m.Set(x, y, Dither(l.Base, 0.6, x, y))
			}
		}
	}
}

// lavaCracks are the cell points of the lava floor, shared by floorLava and
// animLava so they agree.
func lavaCracks(seed uint64) [][2]float64 { return cellPoints(7, seed) }

// floorLava is dark crusted plates with glowing lava (Accent) between them.
func floorLava(m *Indexed, l Look, seed uint64) {
	pts := lavaCracks(seed)
	for y := range TexSize {
		for x := range TexSize {
			c := nearest(float64(x)+0.5, float64(y)+0.5, pts)
			if c.gap() < 2.6 {
				m.SetGlow(x, y, lavaGlow(l.Accent, c.gap(), 0))
				continue
			}
			v := 0.3 + 0.3*(hash2(c.id, 0, seed)-0.5) + 0.3*(tileNoise(float64(x), float64(y), 8, seed+1)-0.5)
			if c.gap() < 4 {
				v += 0.2 // rims lit by the lava
			}
			m.Set(x, y, Dither(l.Base, v, x, y))
		}
	}
}

// lavaGlow is the colour of lava gap pixels from the plate edge: the
// hottest colour (the ramp's last) only in a thin core down the middle of
// a crack, the middle colour for the rest, and the first at the edges.
// heat (0 or 1) lets the wave of heat warm an edge pixel one step; the
// core never grows, so the floor never glares.
func lavaGlow(ramp []color.RGBA, gap float64, heat int) color.RGBA {
	switch {
	case gap < lavaCore:
		return ramp[len(ramp)-1]
	case gap < 1.7:
		return ramp[1]
	}
	return ramp[min(1, heat)]
}

// lavaCore is the half-width of the hottest line down a lava crack.
const lavaCore = 0.25

// animLava makes a slow wave of heat flow along the lava cracks.
func animLava(m *Indexed, l Look, seed uint64, frame int) {
	pts := lavaCracks(seed)
	for y := range TexSize {
		for x := range TexSize {
			c := nearest(float64(x)+0.5, float64(y)+0.5, pts)
			if c.gap() >= 2.6 {
				continue
			}
			phase := tileNoise(float64(x), float64(y), 4, seed+5) + float64(frame)/Frames
			heat := 0
			if math.Mod(phase, 1) < 0.25 {
				heat = 1
			}
			m.SetGlow(x, y, lavaGlow(l.Accent, c.gap(), heat))
		}
	}
}

// floorSnow is soft snow with drifts and a few sparkles.
func floorSnow(m *Indexed, l Look, seed uint64) {
	for y := range TexSize {
		for x := range TexSize {
			v := 0.5 + 0.6*(tileNoise(float64(x), float64(y), 4, seed)-0.5) + 0.2*(tileNoise(float64(x), float64(y), 16, seed+1)-0.5)
			m.Set(x, y, Dither(l.Base, v, x, y))
		}
	}
	rng := NewRand(seed + 2)
	for range 6 {
		m.Set(rng.IntN(TexSize), rng.IntN(TexSize), last(l.Base))
	}
}

// floorGrass is grass with blades; Accent adds a few tiny flowers.
func floorGrass(m *Indexed, l Look, seed uint64) {
	for y := range TexSize {
		for x := range TexSize {
			v := 0.4 + 0.4*(tileNoise(float64(x), float64(y), 8, seed)-0.5)
			m.Set(x, y, Dither(l.Base[:len(l.Base)-1], v, x, y))
		}
	}
	rng := NewRand(seed + 1)
	for range 70 {
		x, y := rng.IntN(TexSize), rng.IntN(TexSize)
		m.Set(x, y, last(l.Base))
		m.Set(x, (y+1)%TexSize, l.Base[len(l.Base)-2])
		m.Set(x, (y+2)%TexSize, l.Base[0])
	}
	for i := 0; i < 4 && len(l.Accent) > 0; i++ {
		m.Set(rng.IntN(TexSize), rng.IntN(TexSize), l.Accent[rng.IntN(len(l.Accent))])
	}
}

// floorPlanks is wooden floorboards with grain and nails.
func floorPlanks(m *Indexed, l Look, seed uint64) {
	for y := range TexSize {
		p, ly := y/8, y%8
		end := int(hash2(p, 0, seed) * TexSize)
		for x := range TexSize {
			if ly == 0 || x == end {
				m.Set(x, y, l.Base[0])
				continue
			}
			if (x == (end+2)%TexSize || x == (end+TexSize-2)%TexSize) && (ly == 2 || ly == 6) {
				m.Set(x, y, last(l.Base)) // a nail
				continue
			}
			v := 0.45 + 0.2*(hash2(p, 1, seed)-0.5)
			v += 0.15 * math.Sin(2*math.Pi*float64(x)/16+hash2(p, 2, seed)*6+float64(ly)*0.9)
			switch ly {
			case 1:
				v += 0.15
			case 7:
				v -= 0.15
			}
			m.Set(x, y, Dither(l.Base[1:], v, x, y))
		}
	}
}

// floorChecker is polished square tiles alternating Base and Accent.
func floorChecker(m *Indexed, l Look, seed uint64) {
	for y := range TexSize {
		for x := range TexSize {
			lx, ly := x%16, y%16
			if lx == 0 || ly == 0 {
				m.Set(x, y, l.Base[0])
				continue
			}
			ramp := l.Base
			if (x/16+y/16)%2 == 1 {
				ramp = l.Accent
			}
			v := 0.5 + 0.08*(tileNoise(float64(x), float64(y), 8, seed)-0.5)
			if d := (lx + ly) % 16; d >= 4 && d <= 6 {
				v += 0.3 // a shine
			}
			if lx == 1 || ly == 1 {
				v += 0.15
			}
			m.Set(x, y, Dither(ramp, v, x, y))
		}
	}
}

// floorGrate is a metal grating over a dark pit.
func floorGrate(m *Indexed, l Look, seed uint64) {
	for y := range TexSize {
		for x := range TexSize {
			lx, ly := x%16, y%16
			border := lx < 2 || ly < 2
			if !border && (lx-2)%5 >= 2 && (ly-2)%5 >= 2 {
				c := pal.Black
				if (lx-2)%5 == 2 && (ly-2)%5 == 2 {
					c = l.Base[0]
				}
				m.Set(x, y, c)
				continue
			}
			v := 0.5
			switch {
			case lx == 1 && ly == 1:
				v = 1
			case lx == 0 || ly == 0:
				v = 0.8
			case !border && ((lx-2)%5 == 1 || (ly-2)%5 == 1):
				v = 0.3
			}
			v += 0.1 * (hash2(x, y, seed) - 0.5)
			m.Set(x, y, Dither(l.Base[1:], v, x, y))
		}
	}
}

// Ceiling families.

// ceilBeams is dark stone blocks crossed by a timber beam. Accent is the
// wood, four colours dark to light.
func ceilBeams(m *Indexed, l Look, seed uint64) {
	wood := l.Accent
	for y := range TexSize {
		for x := range TexSize {
			if y >= 13 && y < 19 {
				c := wood[2]
				switch {
				case y == 13:
					c = wood[3]
				case y == 18:
					c = wood[0]
				case hash2(x/4, y, seed) < 0.3:
					c = wood[1]
				}
				m.Set(x, y, c)
				continue
			}
			if x%16 == 0 || y == 0 {
				m.Set(x, y, l.Base[0])
				continue
			}
			v := 0.4 + 0.3*(ValueNoise(float64(x), float64(y), 5, seed)-0.5)
			m.Set(x, y, Dither(l.Base[1:], v, x, y))
		}
	}
}

// tips draws the points of hanging stalactites or icicles seen from below.
func tips(m *Indexed, ramp []color.RGBA, n int, seed uint64) {
	rng := NewRand(seed)
	for range n {
		cx, cy := rng.IntN(TexSize), rng.IntN(TexSize)
		for dy := -3; dy <= 3; dy++ {
			for dx := -3; dx <= 3; dx++ {
				x, y := wrap(cx+dx, TexSize), wrap(cy+dy, TexSize)
				switch d := hypot(float64(dx), float64(dy)); {
				case d < 1:
					m.Set(x, y, last(ramp))
				case d < 2:
					m.Set(x, y, ramp[len(ramp)-2])
				case d < 3.2:
					m.Set(x, y, ramp[0])
				}
			}
		}
	}
}

// ceilRock is rough rock with stalactite tips.
func ceilRock(m *Indexed, l Look, seed uint64) {
	wallCave(m, l, seed)
	tips(m, l.Base, 3, seed+9)
}

// ceilIce is pale ice with icicle tips coloured by Accent.
func ceilIce(m *Indexed, l Look, seed uint64) {
	for y := range TexSize {
		for x := range TexSize {
			v := 0.45 + 0.5*(tileNoise(float64(x), float64(y), 4, seed)-0.5)
			m.Set(x, y, Dither(l.Base, v, x, y))
		}
	}
	tips(m, l.Accent, 4, seed+9)
}

// ceilPipes is a dark ceiling with flanged pipes (Accent).
func ceilPipes(m *Indexed, l Look, seed uint64) {
	for y := range TexSize {
		for x := range TexSize {
			v := 0.3 + 0.2*(tileNoise(float64(x), float64(y), 8, seed)-0.5)
			if x%16 == 0 {
				v = 0
			}
			m.Set(x, y, Dither(l.Base, v, x, y))
		}
	}
	pipe(m, l.Accent, 5, 6)
	pipe(m, l.Accent, 20, 4)
}

// pipe draws a horizontal pipe w pixels thick from row y0, with flanges.
func pipe(m *Indexed, ramp []color.RGBA, y0, w int) {
	for y := y0; y < y0+w; y++ {
		v := 0.95 - 1.1*math.Abs(float64(y-y0)/float64(w-1)-0.3)
		for x := range TexSize {
			vv := v
			if x%16 == 7 || x%16 == 8 {
				vv += 0.15
			}
			m.Set(x, y, Dither(ramp, vv, x, y))
		}
	}
	for x := range TexSize {
		if x%16 == 6 || x%16 == 9 {
			for y := y0 - 1; y < y0+w+1; y++ {
				m.Set(x, y, ramp[0])
			}
		}
	}
}

// ceilCrystal is crystal facets with glowing veins.
func ceilCrystal(m *Indexed, l Look, seed uint64) {
	wallCrystal(m, l, seed)
	modVeins(m, l, seed+5)
}

// ceilSlabs is long stone slabs.
func ceilSlabs(m *Indexed, l Look, seed uint64) {
	for y := range TexSize {
		for x := range TexSize {
			ly := y % 11
			if ly == 0 || (x == 0 && y < 11) || (x == 16 && y >= 11 && y < 22) || (x == 8 && y >= 22) {
				m.Set(x, y, l.Base[0])
				continue
			}
			v := 0.45 + 0.15*(hash2(y/11, x/16, seed)-0.5) + 0.2*(tileNoise(float64(x), float64(y), 8, seed+1)-0.5)
			switch ly {
			case 1:
				v += 0.2
			case 10:
				v -= 0.2
			}
			m.Set(x, y, Dither(l.Base[1:], v, x, y))
		}
	}
}

// ceilCoffers is a coffered wooden ceiling: Base panels framed by Accent
// beams.
func ceilCoffers(m *Indexed, l Look, seed uint64) {
	for y := range TexSize {
		for x := range TexSize {
			lx, ly := x%16, y%16
			if lx < 3 || ly < 3 {
				v := 0.55
				switch {
				case lx == 0 || ly == 0:
					v = 0.85
				case lx == 2 || ly == 2:
					v = 0.2
				}
				m.Set(x, y, Dither(l.Accent, v, x, y))
				continue
			}
			v := 0.35 + 0.15*(hash2(x, y/2, seed)-0.5)
			if lx == 3 || ly == 3 {
				v -= 0.25
			}
			if lx > 7 && lx < 12 && ly > 7 && ly < 12 {
				v += 0.25 // a raised boss
			}
			m.Set(x, y, Dither(l.Base, v, x, y))
		}
	}
}

// Modifiers.

// modMoss grows moss (Accent) up from the bottom half.
func modMoss(m *Indexed, l Look, seed uint64) {
	for y := TexSize / 2; y < TexSize; y++ {
		for x := range TexSize {
			v := ValueNoise(float64(x), float64(y), 4, seed+9) + float64(y-TexSize/2)/TexSize
			if v > 0.95 {
				m.Set(x, y, pick(l.Accent, hash2(x, y, seed)))
			}
		}
	}
}

// modCrack draws a dark crack down the surface.
func modCrack(m *Indexed, l Look, seed uint64) {
	rng := NewRand(seed)
	x := 8 + rng.IntN(16)
	for y := 2; y < TexSize-4; y++ {
		m.Set(x, y, l.Base[0])
		m.Set(x+1, y, l.Base[1])
		x += rng.IntN(3) - 1
	}
}

// modCobweb spins a web in the top left corner.
func modCobweb(m *Indexed, l Look, seed uint64) {
	for i := range 5 {
		a := float64(i) * math.Pi / 8
		for r := 0.0; r < 12; r += 0.5 {
			m.Set(int(r*math.Cos(a)), int(r*math.Sin(a)), pal.Ash)
		}
	}
	for _, r := range []float64{4, 7.5, 11} {
		for a := 0.0; a < math.Pi/2; a += 0.08 {
			m.Set(int(r*math.Cos(a)), int(r*math.Sin(a)), pal.Ash)
		}
	}
}

// modFrost frosts the top edge.
func modFrost(m *Indexed, l Look, seed uint64) {
	for y := range 12 {
		for x := range TexSize {
			if tileNoise(float64(x), float64(y), 8, seed)-float64(y)/14 > 0.15 {
				m.Set(x, y, pal.White)
			}
		}
	}
	for x := 2; x < TexSize; x += 5 {
		for y := range 3 + int(hash2(x, 0, seed)*7) {
			m.Set(x, y, pal.Ice)
		}
	}
}

// modDrips streaks water (Accent) down the wall.
func modDrips(m *Indexed, l Look, seed uint64) {
	rng := NewRand(seed)
	for range 4 {
		x, n := rng.IntN(TexSize), 8+rng.IntN(20)
		for y := range n {
			m.Set(x, y, pick(l.Accent, 0.5))
		}
		m.Set(x, n, last(l.Accent))
	}
}

// modVeins draws glowing veins (Accent) across the surface.
func modVeins(m *Indexed, l Look, seed uint64) {
	rng := NewRand(seed)
	for range 2 {
		x := float64(rng.IntN(TexSize))
		dx := rng.Float64()*1.4 - 0.7
		for y := range TexSize {
			c := l.Accent[len(l.Accent)-2]
			if rng.IntN(4) == 0 {
				c = last(l.Accent)
			}
			m.SetGlow(wrap(int(x), TexSize), y, c)
			x += dx + rng.Float64()*1.2 - 0.6
			if rng.IntN(9) == 0 {
				dx = -dx
			}
		}
	}
}

// modGear draws a big gear (Accent) in the middle.
func modGear(m *Indexed, l Look, seed uint64) {
	const cx, cy = 16.0, 15.5
	for y := range TexSize {
		for x := range TexSize {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			d, a := hypot(dx, dy), math.Atan2(dy, dx)
			r := 8.5
			if math.Mod(a+math.Pi+0.2, math.Pi/4) < math.Pi/8 {
				r = 11 // a tooth
			}
			switch {
			case d > r:
				continue
			case d < 2:
				m.Set(x, y, pal.Black) // the axle hole
				continue
			case d > 4.5 && d < 7 && math.Mod(a+math.Pi, math.Pi/2) > 0.6:
				m.Set(x, y, l.Base[0]) // holes between the spokes
				continue
			}
			v := 0.55 - 0.25*(dx+dy)/(d+1)
			if d > r-1.2 {
				v -= 0.3
			}
			if d < 5 && d > 3.5 {
				v -= 0.25
			}
			m.Set(x, y, Dither(l.Accent, v, x, y))
		}
	}
}

// modPipe runs a vertical pipe (Accent) down the right side.
func modPipe(m *Indexed, l Look, seed uint64) {
	for y := range TexSize {
		for x := 22; x < 27; x++ {
			v := 0.95 - 1.1*math.Abs(float64(x-22)/4-0.3)
			if y%16 == 4 || y%16 == 5 {
				v += 0.2 // a joint
			}
			m.Set(x, y, Dither(l.Accent, v, x, y))
		}
		m.Set(21, y, l.Base[0])
	}
}

// modFlowers dots the surface with little flowers (Accent).
func modFlowers(m *Indexed, l Look, seed uint64) {
	rng := NewRand(seed)
	for range 9 {
		x, y := rng.IntN(TexSize-2), 3+rng.IntN(TexSize-5)
		c := l.Accent[rng.IntN(len(l.Accent))]
		m.Set(x+1, y, c)
		m.Set(x, y+1, c)
		m.Set(x+2, y+1, c)
		m.Set(x+1, y+2, c)
		m.Set(x+1, y+1, pal.Yellow)
	}
}

// modRaggedTop makes the top edge leafy and see-through, for walls under an
// open sky.
func modRaggedTop(m *Indexed, l Look, seed uint64) {
	for x := range TexSize {
		h := int(1.5 + 3*math.Abs(math.Sin(float64(x)*6*math.Pi/TexSize+hash2(0, 0, seed)*6)) + hash2(x, 0, seed)*1.5)
		for y := range h {
			m.Clear(x, y)
		}
		m.Set(x, h, last(l.Base))
	}
}

// modVines hangs leafy vines (Accent) from the top.
func modVines(m *Indexed, l Look, seed uint64) {
	rng := NewRand(seed)
	for i := range 3 {
		x := 3 + i*10 + rng.IntN(5)
		for y := range 10 + rng.IntN(16) {
			xx := x + int(1.5*math.Sin(float64(y)*0.5))
			m.Set(xx, y, l.Accent[0])
			if y%4 == 2 {
				m.Set(xx-1, y, last(l.Accent))
				m.Set(xx+1, y+1, pick(l.Accent, 0.5))
			}
		}
	}
}

// modSoot blackens the top of the wall.
func modSoot(m *Indexed, l Look, seed uint64) {
	for y := range 16 {
		for x := range TexSize {
			if tileNoise(float64(x), float64(y), 8, seed)-float64(y)/18 > 0.2 && bayer4[y&3][x&3] < 0.6 {
				m.Set(x, y, pal.Black)
			}
		}
	}
}

// modGlowCracks breaks the surface with glowing cracks (Accent).
func modGlowCracks(m *Indexed, l Look, seed uint64) {
	rng := NewRand(seed)
	x := 6 + rng.IntN(20)
	for y := range TexSize {
		m.SetGlow(x, y, pick(l.Accent, 0.6))
		if rng.IntN(3) == 0 {
			m.SetGlow(x+1, y, last(l.Accent))
		}
		x += rng.IntN(3) - 1
	}
}

// modSandDrift piles sand against the bottom of the wall.
func modSandDrift(m *Indexed, l Look, seed uint64) {
	for x := range TexSize {
		h := 3 + int(4*tileNoise(float64(x), 0, 4, seed))
		for y := TexSize - h; y < TexSize; y++ {
			v := 0.7 - 0.4*float64(y-(TexSize-h))/float64(h)
			m.Set(x, y, Dither(sandRamp, v, x, y))
		}
	}
}

// sandRamp colours drifting sand.
var sandRamp = []color.RGBA{pal.Brown, pal.Bronze, pal.Tan, pal.Skin}
