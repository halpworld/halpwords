package scene

import (
	"image/color"

	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/pal"
)

// Effects that make things feel solid: sparks off a blow, coins from a
// chest, a pause on a critical hit. They only change how things look.

// viewMid is the middle of the 3D view, in screen pixels, where the
// monster in a battle stands.
func viewMid() (float64, float64) {
	return viewX + viewW*gfx.ArtScale/2, viewY + viewH*gfx.ArtScale/2 + 12
}

// viewFloor is the bottom middle of the view, where things rise from.
func viewFloor() (float64, float64) {
	return viewX + viewW*gfx.ArtScale/2, viewY + viewH*gfx.ArtScale - 8
}

var (
	hitColors   = []color.RGBA{pal.White, pal.Yellow, pal.Orange}
	critColors  = []color.RGBA{pal.White, pal.Yellow, pal.Yellow, pal.Orange, pal.Red}
	weakColors  = []color.RGBA{pal.Steel, pal.Ash, pal.White}
	coinColors  = []color.RGBA{pal.Yellow, pal.Yellow, pal.Bronze, pal.White}
	runeColors  = []color.RGBA{pal.Cyan, pal.Sky, pal.White, pal.Ice}
	healColors  = []color.RGBA{pal.Lime, pal.Green, pal.White}
	levelColors = []color.RGBA{pal.Yellow, pal.White, pal.Lime, pal.Orange}
	dustColors  = []color.RGBA{pal.Plum, pal.Pink, pal.White, pal.Ash}
)

// fxHit throws sparks off the monster: more, and faster, for a critical
// hit, which also stops time for a moment.
func (c *Crawl) fxHit(crit, weak bool) {
	x, y := viewMid()
	switch {
	case crit:
		c.fx.Burst(x, y, gfx.Burst{N: 30, Speed: 5, Fall: 0.15, Life: 26, Colors: critColors})
		c.freeze = 5
		c.shake = max(c.shake, 6)
	case weak:
		c.fx.Burst(x, y, gfx.Burst{N: 6, Speed: 2, Fall: 0.1, Life: 14, Colors: weakColors})
	default:
		c.fx.Burst(x, y, gfx.Burst{N: 14, Speed: 3.5, Fall: 0.15, Life: 20, Colors: hitColors})
	}
}

// fxClang is a blow turned aside by armour.
func (c *Crawl) fxClang() {
	x, y := viewMid()
	c.fx.Burst(x, y-10, gfx.Burst{N: 10, Speed: 4, Fall: 0.25, Life: 16, Colors: weakColors})
}

// fxDefeat is a monster breaking up into dust.
func (c *Crawl) fxDefeat() {
	x, y := viewMid()
	c.fx.Burst(x, y, gfx.Burst{N: 36, Speed: 2.2, Fall: -0.03, Life: 40, Colors: dustColors})
}

// fxCoins is a fountain of gold.
func (c *Crawl) fxCoins(gold int) {
	if gold <= 0 {
		return
	}
	x, y := viewMid()
	c.fx.Burst(x, y+20, gfx.Burst{N: max(6, min(24, gold/2)), Speed: 4.5, Fall: 0.28, Life: 36, Up: true, Colors: coinColors})
}

// fxRunes is a seal breaking, or a rune being read.
func (c *Crawl) fxRunes() {
	x, y := viewMid()
	c.fx.Burst(x, y-8, gfx.Burst{N: 32, Speed: 3.5, Fall: 0.02, Life: 34, Colors: runeColors})
}

// fxHeal is bubbles of light rising around the hero.
func (c *Crawl) fxHeal() {
	x, y := viewFloor()
	c.fx.Burst(x, y, gfx.Burst{N: 20, Speed: 2.5, Fall: -0.02, Life: 44, Up: true, Colors: healColors})
}

// fxLevelUp is a shower of light for a new level.
func (c *Crawl) fxLevelUp() {
	x, y := viewFloor()
	c.fx.Burst(x-90, y, gfx.Burst{N: 24, Speed: 5, Fall: 0.08, Life: 60, Up: true, Colors: levelColors})
	c.fx.Burst(x+90, y, gfx.Burst{N: 24, Speed: 5, Fall: 0.08, Life: 60, Up: true, Colors: levelColors})
}
