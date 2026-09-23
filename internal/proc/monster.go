package proc

import (
	"image/color"
	"math"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/pal"
)

// MonsterSize is the width and height of a monster sprite.
const MonsterSize = 32

// Sprite parts. Monsters are sketched as parts on a grid, then shaded.
const (
	partNone   = iota
	partBody   // shaded with the main ramp
	partAccent // shaded with the accent pair
	partDark   // mouths, sockets
	partWhite  // teeth, eye whites
	partGlow   // glowing eyes
	partPupil  // black
)

// monsterLook is the colouring for one Kind.Hue.
type monsterLook struct {
	ramp   []color.RGBA // body, dark to light
	accent [2]color.RGBA
	glow   color.RGBA
}

var monsterLooks = []monsterLook{
	{[]color.RGBA{pal.Forest, pal.Green, pal.Lime, pal.Yellow}, [2]color.RGBA{pal.Olive, pal.Forest}, pal.Yellow}, // 0 green slime
	{[]color.RGBA{pal.Night, pal.Plum, pal.Purple, pal.Pink}, [2]color.RGBA{pal.Plum, pal.Rose}, pal.Red},         // 1 bat
	{[]color.RGBA{pal.Plum, pal.Mahogany, pal.Brown, pal.Tan}, [2]color.RGBA{pal.Rose, pal.Pink}, pal.Red},        // 2 rat
	{[]color.RGBA{pal.Granite, pal.Stone, pal.Ash, pal.Ice}, [2]color.RGBA{pal.Bronze, pal.Tan}, pal.Red},         // 3 bones
	{[]color.RGBA{pal.Navy, pal.Blue, pal.Sky, pal.Ice}, [2]color.RGBA{pal.Navy, pal.Cyan}, pal.Cyan},             // 4 wisp
	{[]color.RGBA{pal.Plum, pal.Red, pal.Rose, pal.Pink}, [2]color.RGBA{pal.Teal, pal.Lime}, pal.Yellow},          // 5 gazer
	{[]color.RGBA{pal.Black, pal.Night, pal.Slate, pal.Granite}, [2]color.RGBA{pal.Red, pal.Rose}, pal.Red},       // 6 spider
	{[]color.RGBA{pal.Navy, pal.Blue, pal.Sky, pal.Cyan}, [2]color.RGBA{pal.Indigo, pal.Navy}, pal.Yellow},        // 7 blue slime
	{[]color.RGBA{pal.Slate, pal.Granite, pal.Stone, pal.Ash}, [2]color.RGBA{pal.Forest, pal.Green}, pal.Yellow},  // 8 moss golem
	{[]color.RGBA{pal.Mahogany, pal.Red, pal.Orange, pal.Yellow}, [2]color.RGBA{pal.Night, pal.Plum}, pal.Yellow}, // 9 fire imp
	{[]color.RGBA{pal.Indigo, pal.Purple, pal.Steel, pal.Ice}, [2]color.RGBA{pal.Navy, pal.Cyan}, pal.Cyan},       // 10 mirror imp
	{[]color.RGBA{pal.Plum, pal.Mahogany, pal.Brown, pal.Tan}, [2]color.RGBA{pal.Rose, pal.Pink}, pal.Yellow},     // 11 mimic
}

// canvas is a grid of parts.
type canvas [MonsterSize * MonsterSize]uint8

func (c *canvas) at(x, y int) uint8 {
	if x < 0 || y < 0 || x >= MonsterSize || y >= MonsterSize {
		return partNone
	}
	return c[y*MonsterSize+x]
}

func (c *canvas) set(x, y int, p uint8) {
	if x >= 0 && y >= 0 && x < MonsterSize && y < MonsterSize {
		c[y*MonsterSize+x] = p
	}
}

// fill sets every pixel whose centre satisfies in.
func (c *canvas) fill(p uint8, in func(x, y float64) bool) {
	for y := 0; y < MonsterSize; y++ {
		for x := 0; x < MonsterSize; x++ {
			if in(float64(x)+0.5, float64(y)+0.5) {
				c[y*MonsterSize+x] = p
			}
		}
	}
}

func (c *canvas) ellipse(cx, cy, rx, ry float64, p uint8) {
	c.fill(p, func(x, y float64) bool {
		dx, dy := (x-cx)/rx, (y-cy)/ry
		return dx*dx+dy*dy <= 1
	})
}

func (c *canvas) disc(cx, cy, r float64, p uint8) { c.ellipse(cx, cy, r, r, p) }

func (c *canvas) rect(x, y, w, h int, p uint8) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			c.set(i, j, p)
		}
	}
}

// line draws a segment of the given thickness.
func (c *canvas) line(x0, y0, x1, y1, width float64, p uint8) {
	dx, dy := x1-x0, y1-y0
	l2 := dx*dx + dy*dy
	c.fill(p, func(x, y float64) bool {
		t := 0.0
		if l2 > 0 {
			t = math.Max(0, math.Min(1, ((x-x0)*dx+(y-y0)*dy)/l2))
		}
		return math.Hypot(x-x0-t*dx, y-y0-t*dy) <= width/2
	})
}

func (c *canvas) tri(ax, ay, bx, by, cx, cy float64, p uint8) {
	side := func(px, py, qx, qy, x, y float64) float64 { return (qx-px)*(y-py) - (qy-py)*(x-px) }
	c.fill(p, func(x, y float64) bool {
		d1, d2, d3 := side(ax, ay, bx, by, x, y), side(bx, by, cx, cy, x, y), side(cx, cy, ax, ay, x, y)
		neg := d1 < 0 || d2 < 0 || d3 < 0
		pos := d1 > 0 || d2 > 0 || d3 > 0
		return !(neg && pos)
	})
}

// eye draws a white eye with a pupil looking slightly down.
func (c *canvas) eye(cx, cy, r float64) {
	c.disc(cx, cy, r, partWhite)
	c.disc(cx, cy+r*0.3, r*0.55, partPupil)
}

// MonsterSprite draws a monster of the given family and colour. frame (0 or 1)
// gives a two-step idle animation; seed varies the details.
func MonsterSprite(fam dungeon.Family, hue int, seed uint64, frame int) *Indexed {
	var c canvas
	rng := NewRand(seed)
	j := func(n float64) float64 { return (rng.Float64()*2 - 1) * n } // jitter
	f := float64(frame & 1)

	switch fam {
	case dungeon.Slime:
		sq := f * 1.5 // squash
		c.ellipse(16, 22+sq/2, 13+sq, 9-sq/2, partBody)
		c.tri(16+j(2), 6+sq, 7, 20, 25, 20, partBody)
		c.disc(10, 28, 3, partBody) // drips
		c.disc(23, 29, 2, partBody)
		ex := 5 + j(1)
		c.eye(16-ex, 19+sq, 2.6)
		c.eye(16+ex, 19+sq, 2.6)
		c.ellipse(16, 25+sq, 4, 1.5, partDark)
		c.rect(10, 13+int(sq), 2, 2, partWhite) // shine
	case dungeon.Bat:
		up := 5 * f
		for _, s := range []float64{-1, 1} {
			tip := 16 + s*15
			c.tri(16, 12, tip, 5+up, 16+s*4, 21, partBody)
			c.tri(16+s*5, 15, tip, 5+up, 16+s*12, 18+up/2, partBody)
			// Scallops cut out of the trailing edge.
			c.disc(16+s*8, 21+up/3, 2.5, partNone)
			c.disc(16+s*12.5, 19+up/2, 2, partNone)
			c.tri(16+s*2, 8, 16+s*6, 3, 16+s*6, 11, partBody) // ears
			c.line(16+s*3, 13, tip, 5+up, 1, partAccent)      // wing bone
		}
		c.ellipse(16, 15, 5, 6, partBody)
		c.disc(14, 13, 1.2, partGlow)
		c.disc(18, 13, 1.2, partGlow)
		c.set(14, 18, partWhite)
		c.set(17, 18, partWhite)
		c.rect(15, 17, 2, 1, partDark)
	case dungeon.Rat:
		c.ellipse(16, 24, 11, 7, partBody)
		c.line(24, 27, 31, 21+f*2, 1.5, partAccent) // tail
		for _, s := range []float64{-1, 1} {
			c.disc(16+s*7, 8, 4, partBody)
			c.disc(16+s*7, 8, 2.2, partAccent)
		}
		c.disc(16, 14, 6.5, partBody)
		c.ellipse(16, 18.5, 3.5, 3, partBody)
		c.disc(16, 17, 1.3, partAccent) // nose
		c.disc(13, 13, 1.2, partGlow)
		c.disc(19, 13, 1.2, partGlow)
		c.rect(15, 21, 2, 2, partWhite) // buck teeth
		for _, s := range []float64{-1, 1} {
			c.line(16+s*4, 19, 16+s*10, 18+f, 0.8, partDark) // whiskers
			c.line(16+s*4, 20, 16+s*10, 21, 0.8, partDark)
		}
		c.rect(9, 29, 4, 2, partAccent) // paws
		c.rect(19, 29, 4, 2, partAccent)
	case dungeon.Skull:
		c.rect(15, 20, 3, 11, partBody) // spine
		for i, y := range []float64{22, 25, 28} {
			w := 7.0 - float64(i)
			c.line(16-w, y+1, 16+w, y+1, 1.5, partBody)
			c.line(16-w, y+1, 16-w, y+2.5, 1.5, partBody)
			c.line(16+w, y+1, 16+w, y+2.5, 1.5, partBody)
		}
		sw := f * 2
		c.line(8, 21, 4, 29-sw, 2, partBody) // arms
		c.line(24, 21, 28, 29-sw, 2, partBody)
		c.disc(16, 10, 8, partBody)
		c.rect(11, 14, 11, 5, partBody)
		c.disc(12.5, 10, 2.6, partDark)
		c.disc(19.5, 10, 2.6, partDark)
		c.disc(12.5, 10.5, 0.9, partGlow)
		c.disc(19.5, 10.5, 0.9, partGlow)
		c.tri(16, 13, 14.5, 16, 17.5, 16, partDark)
		for x := 12; x < 21; x += 2 {
			c.set(x, 18, partDark)
		}
	case dungeon.Ghost:
		c.disc(16, 12, 10, partBody)
		c.fill(partBody, func(x, y float64) bool {
			return x > 6 && x < 26 && y > 12 && y < 26+2*math.Sin(x*0.9+f*2)
		})
		c.line(7, 16, 2, 20-f*3, 3, partBody) // arms
		c.line(25, 16, 30, 20-f*3, 3, partBody)
		c.ellipse(12, 11, 2, 3, partDark)
		c.ellipse(20, 11, 2, 3, partDark)
		c.disc(12, 11, 1, partGlow)
		c.disc(20, 11, 1, partGlow)
		c.ellipse(16, 18, 2, 2.5+f, partDark)
	case dungeon.Eye:
		for i := 0; i < 4; i++ {
			x := 7 + float64(i)*6 + j(1)
			top := 2 + j(1.5) + f*float64(i%2)
			c.line(16, 10, x, top+2, 1.5, partAccent)
			c.disc(x, top+1, 2, partBody)
			c.set(int(x), int(top+1), partGlow)
		}
		c.disc(16, 17, 11, partBody)
		c.disc(16, 15, 6.5, partWhite)
		c.disc(16+f, 15.5, 3.8, partAccent)
		c.disc(16+f, 15.5, 1.8, partPupil)
		c.set(14, 13, partWhite)
		c.ellipse(16, 25, 5, 1.8, partDark)
		for x := 12; x <= 20; x += 2 {
			c.set(x, 24, partWhite)
		}
	case dungeon.Spider:
		for _, s := range []float64{-1, 1} {
			for i := 0; i < 4; i++ {
				k := float64(i)
				bob := 0.0
				if (i+frame)%2 == 0 {
					bob = 1
				}
				kx, ky := 16+s*(9+k*1.2), 8+k*4-bob
				c.line(16+s*3, 15+k*1.5, kx, ky, 1.5, partBody)
				c.line(kx, ky, 16+s*(12+k*1.1), 22+k*3, 1.5, partBody)
			}
		}
		c.ellipse(16, 20, 8, 7, partBody)
		c.disc(16, 11, 4.5, partBody)
		// Hourglass mark.
		c.tri(13.5, 17, 18.5, 17, 16, 20, partAccent)
		c.tri(13.5, 23, 18.5, 23, 16, 20, partAccent)
		for _, e := range [][2]int{{14, 10}, {17, 10}, {13, 12}, {18, 12}} {
			c.set(e[0], e[1], partGlow)
		}
		c.rect(14, 14, 1, 2, partWhite) // fangs
		c.rect(17, 14, 1, 2, partWhite)
	case dungeon.Golem:
		sw := int(f)
		c.rect(1, 10+sw, 6, 13, partBody) // arms
		c.rect(25, 10+sw, 6, 13, partBody)
		c.rect(0, 22+sw, 8, 6, partBody) // fists
		c.rect(24, 22+sw, 8, 6, partBody)
		c.rect(9, 26, 6, 6, partBody) // legs
		c.rect(17, 26, 6, 6, partBody)
		c.rect(8, 9, 16, 18, partBody)      // torso
		c.line(13, 13, 17, 19, 1, partDark) // crack
		c.line(17, 19, 15, 24, 1, partDark)
		for i := 0; i < 7; i++ {
			c.disc(4+rng.Float64()*24, 9+rng.Float64()*18, 1.5+rng.Float64(), partAccent)
		}
		c.rect(8, 26, 16, 1, partNone) // gap under the torso
		c.rect(11, 1, 10, 7, partBody) // head, drawn after the moss
		c.rect(10, 8, 12, 1, partNone) // neck gap
		c.rect(12, 3, 8, 3, partDark)
		c.rect(13, 4, 2, 1, partGlow)
		c.rect(17, 4, 2, 1, partGlow)
	case dungeon.Mimic:
		// A chest whose lid chomps open and shut.
		open := 5 + 2*f
		top := 18 - open // top of the mouth
		c.rect(3, 18, 26, 12, partBody)
		c.rect(3, 24, 26, 1, partDark) // plank line
		c.rect(4, int(top), 24, int(open), partDark)
		c.ellipse(16, top-4, 13, 2.5, partBody) // curved lid
		c.rect(3, int(top)-4, 26, 4, partBody)
		for x := 5.0; x < 26; x += 4 {
			c.tri(x, 18, x+3, 18, x+1.5, 15.5, partWhite)        // bottom teeth
			c.tri(x+2, top, x+5, top, x+3.5, top+2.5, partWhite) // top teeth
		}
		c.rect(8, int(top)-3, 4, 1, partDark) // angry brows
		c.rect(20, int(top)-3, 4, 1, partDark)
		c.disc(10, top-1.5, 1.3, partGlow)
		c.disc(22, top-1.5, 1.3, partGlow)
		c.ellipse(21+j(1), 20.5, 3, 4, partAccent) // tongue
		c.line(21, 18, 21, 23, 0.8, partDark)
		c.disc(12, 21.5, 1.5, partDark) // keyhole
		c.rect(11, 22, 2, 3, partDark)
	default: // Imp
		for _, s := range []float64{-1, 1} {
			c.tri(16+s*3, 6, 16+s*7, 0, 16+s*6, 7, partAccent)         // horns
			c.tri(16+s*6, 10, 16+s*12, 7, 16+s*6, 14, partBody)        // ears
			c.line(16+s*5, 19, 16+s*10, 23-f*4, 2.5, partBody)         // arms
			c.line(16+s*3, 27, 16+s*4, 31, 2.5, partBody)              // legs
			c.tri(16+s*6, 17, 16+s*14, 12+f*2, 16+s*8, 22, partAccent) // wings
		}
		c.line(21, 27, 27, 29, 1.2, partBody) // tail
		c.tri(27, 27, 30, 29, 27, 31, partBody)
		c.ellipse(16, 23, 6, 6, partBody)
		c.disc(16, 11, 7, partBody)
		c.disc(13, 10, 1.4, partGlow)
		c.disc(19, 10, 1.4, partGlow)
		c.ellipse(16, 14.5, 3.5, 1.3, partDark)
		c.set(14, 14, partWhite)
		c.set(18, 14, partWhite)
	}
	return shadeMonster(&c, monsterLooks[hue%len(monsterLooks)])
}

// shadeMonster colours a part grid: bodies get lit from the top left, with a
// rim light on the lit edge and a darker bottom-right edge.
func shadeMonster(c *canvas, look monsterLook) *Indexed {
	const n = MonsterSize
	// Distance from each body pixel to the nearest non-body pixel.
	depth := make([]int, n*n)
	var queue []int
	for i, p := range c {
		if p == partBody || p == partAccent {
			depth[i] = -1
		} else {
			queue = append(queue, i)
		}
	}
	for len(queue) > 0 {
		i := queue[0]
		queue = queue[1:]
		x, y := i%n, i/n
		for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nx, ny := x+d[0], y+d[1]
			if nx < 0 || ny < 0 || nx >= n || ny >= n {
				continue
			}
			if k := ny*n + nx; depth[k] < 0 {
				depth[k] = depth[i] + 1
				queue = append(queue, k)
			}
		}
	}

	m := NewIndexed(n, n)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			switch c.at(x, y) {
			case partBody:
				v := 0.2 + 0.14*float64(min(depth[y*n+x], 4))
				if c.at(x-1, y) == partNone || c.at(x, y-1) == partNone {
					v += 0.35
				}
				if c.at(x+1, y) == partNone || c.at(x, y+1) == partNone {
					v -= 0.2
				}
				v -= 0.25 * float64(y-n/2) / n
				m.Set(x, y, Dither(look.ramp, v, x, y))
			case partAccent:
				v := 0.3 + 0.2*float64(min(depth[y*n+x], 2))
				if c.at(x-1, y) == partNone || c.at(x, y-1) == partNone {
					v += 0.3
				}
				m.Set(x, y, Dither(look.accent[:], v, x, y))
			case partDark:
				m.Set(x, y, pal.Night)
			case partWhite:
				m.Set(x, y, pal.White)
			case partGlow:
				m.SetGlow(x, y, look.glow)
			case partPupil:
				m.Set(x, y, pal.Black)
			}
		}
	}
	outline(m)
	return m
}
