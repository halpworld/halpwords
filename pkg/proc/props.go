package proc

import (
	"image/color"
	"math"

	"github.com/halpworld/halpwords/internal/pal"
)

// Props are the decorations standing on a world's floor. They are sketched
// on a part grid and shaded like monsters, so they share the monsters' look.

// PropSizePx is the width and height of a prop sprite.
const PropSizePx = MonsterSize

// shadeProp colours a prop's part grid.
func shadeProp(c *canvas, ramp []color.RGBA, accent [2]color.RGBA, glow color.RGBA) *Indexed {
	return shadeMonster(c, monsterLook{ramp: ramp, accent: accent, glow: glow})
}

// PropBones is a skull on a heap of bones.
func PropBones(seed uint64) *Indexed {
	var c canvas
	c.line(5, 29, 26, 25, 2.2, partBody)
	c.line(7, 25, 25, 30, 2.2, partBody)
	c.disc(5, 29, 2, partBody)
	c.disc(26, 25, 2, partBody)
	c.disc(16, 21, 6, partBody)
	c.rect(13, 25, 7, 3, partBody)
	c.disc(13.5, 20.5, 1.6, partDark)
	c.disc(18.5, 20.5, 1.6, partDark)
	c.rect(15, 23, 2, 1, partDark)
	return shadeProp(&c, []color.RGBA{pal.Granite, pal.Ash, pal.Steel, pal.Ice}, [2]color.RGBA{pal.Ash, pal.Ice}, pal.Red)
}

// PropMushrooms is a cluster of glowing mushrooms.
func PropMushrooms(seed uint64) *Indexed {
	var c canvas
	c.rect(9, 20, 3, 11, partAccent)
	c.rect(20, 23, 2, 8, partAccent)
	c.ellipse(10.5, 19, 7, 4.5, partBody)
	c.ellipse(21, 22.5, 5, 3.5, partBody)
	c.rect(4, 19, 14, 2, partBody)
	m := shadeProp(&c, []color.RGBA{pal.Navy, pal.Teal, pal.Cyan, pal.Ice}, [2]color.RGBA{pal.Tan, pal.Skin}, pal.Lime)
	for _, p := range [][2]int{{8, 16}, {12, 17}, {10, 15}, {6, 18}, {20, 21}, {23, 21}} {
		m.SetGlow(p[0], p[1], pal.Lime)
	}
	return m
}

// PropStalagmite is pointed rock rising from the floor.
func PropStalagmite(seed uint64) *Indexed {
	var c canvas
	c.tri(15, 3, 8, 31, 23, 31, partBody)
	c.tri(23, 15, 19, 31, 28, 31, partBody)
	c.tri(7, 20, 4, 31, 11, 31, partBody)
	return shadeProp(&c, []color.RGBA{pal.Night, pal.Indigo, pal.Navy, pal.Teal, pal.Cyan}, [2]color.RGBA{}, pal.Cyan)
}

// PropIce is a snow pile with ice shards.
func PropIce(seed uint64) *Indexed {
	var c canvas
	c.tri(12, 6, 8, 26, 16, 26, partAccent)
	c.tri(20, 11, 17, 26, 24, 26, partAccent)
	c.tri(25, 17, 22, 27, 28, 27, partAccent)
	c.ellipse(16, 29, 13, 4, partBody)
	return shadeProp(&c, []color.RGBA{pal.Steel, pal.Ice, pal.White}, [2]color.RGBA{pal.Navy, pal.Cyan}, pal.Cyan)
}

// PropAnvil is an anvil with a glowing hot bar on it.
func PropAnvil(seed uint64) *Indexed {
	var c canvas
	c.rect(8, 15, 16, 4, partBody)
	c.tri(24, 15, 31, 15, 24, 18, partBody)
	c.rect(12, 19, 8, 6, partBody)
	c.rect(9, 25, 14, 5, partBody)
	c.rect(10, 13, 10, 2, partGlow)
	return shadeProp(&c, []color.RGBA{pal.Black, pal.Night, pal.Slate, pal.Granite, pal.Stone}, [2]color.RGBA{}, pal.Orange)
}

// PropCrystals is a cluster of glowing crystals.
func PropCrystals(seed uint64) *Indexed {
	var c canvas
	c.tri(15, 2, 11, 30, 20, 30, partBody)
	c.tri(9, 12, 4, 30, 12, 30, partBody)
	c.tri(23, 9, 18, 30, 27, 30, partBody)
	c.line(15, 8, 15, 26, 1, partGlow)
	return shadeProp(&c, []color.RGBA{pal.Plum, pal.Purple, pal.Pink, pal.Skin}, [2]color.RGBA{}, pal.Pink)
}

// PropCog is a big cog leaning on a crate.
func PropCog(seed uint64) *Indexed {
	var c canvas
	c.rect(4, 22, 12, 9, partAccent)
	for i := range 8 {
		a := float64(i) * math.Pi / 4
		c.disc(19+9*math.Cos(a), 19+9*math.Sin(a), 2.2, partBody)
	}
	c.disc(19, 19, 8.5, partBody)
	c.disc(19, 19, 3, partDark)
	return shadeProp(&c, []color.RGBA{pal.Mahogany, pal.Bronze, pal.Tan, pal.Yellow}, [2]color.RGBA{pal.Mahogany, pal.Brown}, pal.Yellow)
}

// PropFlowers is a round flowering bush.
func PropFlowers(seed uint64) *Indexed {
	var c canvas
	c.ellipse(16, 22, 12, 9, partBody)
	c.ellipse(10, 25, 7, 6, partBody)
	m := shadeProp(&c, []color.RGBA{pal.Forest, pal.Green, pal.Lime}, [2]color.RGBA{}, pal.Yellow)
	rng := NewRand(seed)
	cols := []color.RGBA{pal.Pink, pal.Rose, pal.Yellow, pal.White}
	for range 11 {
		x, y := 7+rng.IntN(18), 15+rng.IntN(12)
		if m.At(x, y) == Transparent || m.At(x, y) == 0 {
			continue // off the bush, or on its outline
		}
		col := cols[rng.IntN(len(cols))]
		m.Set(x, y, col)
		m.Set(x+1, y, col)
		m.Set(x, y+1, col)
		m.Set(x+1, y+1, pal.Yellow)
	}
	return m
}

// PropUrn is a clay urn with a turquoise band.
func PropUrn(seed uint64) *Indexed {
	var c canvas
	c.ellipse(16, 21, 8, 9, partBody)
	c.rect(12, 8, 8, 6, partBody)
	c.rect(10, 7, 12, 2, partBody)
	c.rect(12, 29, 8, 2, partBody)
	c.line(9, 12, 7, 18, 1.5, partBody)
	c.line(23, 12, 25, 18, 1.5, partBody)
	c.rect(9, 19, 15, 2, partAccent)
	return shadeProp(&c, []color.RGBA{pal.Plum, pal.Mahogany, pal.Brown, pal.Tan, pal.Skin}, [2]color.RGBA{pal.Teal, pal.Cyan}, pal.Yellow)
}

// PropBooks is a pile of books with a candle on top.
func PropBooks(seed uint64) *Indexed {
	m := NewIndexed(PropSizePx, PropSizePx)
	books := []struct {
		x, y, w     int
		dark, light color.RGBA
	}{
		{4, 26, 24, pal.Navy, pal.Blue},
		{6, 22, 19, pal.Red, pal.Rose},
		{3, 18, 21, pal.Forest, pal.Green},
		{8, 14, 16, pal.Purple, pal.Pink},
		{11, 10, 10, pal.Mahogany, pal.Brown},
	}
	for _, b := range books {
		m.Rect(b.x, b.y, b.w, 4, b.dark)
		m.Rect(b.x, b.y, b.w, 1, b.light)
		m.Rect(b.x+b.w-3, b.y+1, 3, 2, pal.Skin) // the pages
		m.Set(b.x+2, b.y+2, pal.Yellow)
	}
	m.Rect(14, 6, 4, 4, pal.Ice) // the candle
	m.Rect(15, 7, 2, 3, pal.White)
	m.SetGlow(15, 4, pal.Orange)
	m.SetGlow(16, 3, pal.Yellow)
	m.SetGlow(15, 5, pal.Yellow)
	outline(m)
	return m
}
