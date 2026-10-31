package raycast

import (
	"image/color"
	"math"
	"testing"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/pkg/proc"
)

var extremeLights = []float64{
	math.NaN(), math.Inf(1), math.Inf(-1), -1, -1e308, 1e308, 5e-324, 0, math.Copysign(0, -1),
	0.35, 0.999999, 1, 1.000001, 2, 1e-9,
}

// Every world and its remixes, at extreme sprite light, sets up and draws
// a frame without panicking, and keeps the sprite light inside [0,1].
func TestQAEveryThemeAtExtremeSpriteLight(t *testing.T) {
	r := New(64, 40)
	lvl := dungeon.Generate(77, 3)
	for i := range proc.Themes {
		for lap := range 3 {
			base := proc.Themes[i].Remix(lap)
			for _, v := range extremeLights {
				th := *base
				th.SpriteLight = v
				r.SetWorld(&th)
				if !(r.spriteLight >= 0 && r.spriteLight <= 1) || r.spriteLevel < 0 || r.spriteLevel >= levels {
					t.Fatalf("%s lap %d light %v: light %v level %d", th.Name, lap, v, r.spriteLight, r.spriteLevel)
				}
				tex := NewTextures(&th, lvl.Seed)
				r.Render(lvl, tex, startCamera(lvl), Decor(lvl, &th), uint64(i*7+lap))
				r.DrawParticles(startCamera(lvl), 5)
			}
		}
	}
}

// validShade checks the sprite table only holds palette indices.
func validShade(t *testing.T, r *Renderer, what string) {
	t.Helper()
	for lv := range levels {
		for c := range len(pal.All) {
			if int(r.spriteShade[lv][c]) >= len(pal.All) && r.spriteShade[lv][c] != proc.Transparent {
				t.Errorf("%s: level %d colour %d shades to %d", what, lv, c, r.spriteShade[lv][c])
			}
		}
	}
}

// Swap lists of any shape: empty, identical pair, colours off the palette,
// transparent black, duplicates, chains, cycles, every colour at once.
func TestQASpriteSwapShapes(t *testing.T) {
	plain := New(32, 20)
	base := proc.Themes[0]
	plain.SetWorld(&base)
	p := func(c color.RGBA) uint8 { return pal.Index(c) }
	swapOf := func(pairs ...[2]color.RGBA) *Renderer {
		r := New(32, 20)
		th := base
		th.SpriteSwap = pairs
		r.SetWorld(&th)
		validShade(t, r, "swap")
		return r
	}
	// Nothing, or a colour swapped for itself, changes nothing.
	for name, r := range map[string]*Renderer{
		"nil":      swapOf(),
		"empty":    swapOf([][2]color.RGBA{}...),
		"identity": swapOf([2]color.RGBA{pal.Purple, pal.Purple}),
	} {
		if r.spriteShade != plain.spriteShade {
			t.Errorf("%s changed the table", name)
		}
	}
	// A chain A→B, B→C: A shades as B did before swapping, not as C.
	chain := swapOf([2]color.RGBA{pal.Purple, pal.Pink}, [2]color.RGBA{pal.Pink, pal.Ice})
	for lv := range levels {
		if got, want := chain.spriteShade[lv][p(pal.Purple)], plain.spriteShade[lv][p(pal.Pink)]; got != want {
			t.Errorf("chain: purple at level %d is %d, want the plain pink %d", lv, got, want)
		}
		if got, want := chain.spriteShade[lv][p(pal.Pink)], plain.spriteShade[lv][p(pal.Ice)]; got != want {
			t.Errorf("chain: pink at level %d is %d, want the plain ice %d", lv, got, want)
		}
	}
	// A cycle swaps, rather than both ending up the same.
	cyc := swapOf([2]color.RGBA{pal.Purple, pal.Pink}, [2]color.RGBA{pal.Pink, pal.Purple})
	for lv := range levels {
		if cyc.spriteShade[lv][p(pal.Purple)] != plain.spriteShade[lv][p(pal.Pink)] ||
			cyc.spriteShade[lv][p(pal.Pink)] != plain.spriteShade[lv][p(pal.Purple)] {
			t.Errorf("cycle at level %d did not exchange the colours", lv)
		}
	}
	// Off-palette and transparent colours map to the nearest palette entry
	// and never leave the table broken.
	swapOf([2]color.RGBA{{1, 2, 3, 255}, {250, 251, 252, 255}})
	swapOf([2]color.RGBA{{}, pal.Pink}, [2]color.RGBA{pal.Pink, {}})
	swapOf([2]color.RGBA{{R: 255, A: 0}, {B: 255, A: 0}})
	// Duplicates: the last of the same source wins, deterministically.
	d1 := swapOf([2]color.RGBA{pal.Purple, pal.Pink}, [2]color.RGBA{pal.Purple, pal.Ice})
	d2 := swapOf([2]color.RGBA{pal.Purple, pal.Pink}, [2]color.RGBA{pal.Purple, pal.Ice})
	if d1.spriteShade != d2.spriteShade {
		t.Error("duplicate swaps are not deterministic")
	}
	// Every palette colour swapped for its neighbour at once.
	var all [][2]color.RGBA
	for i, c := range pal.All {
		all = append(all, [2]color.RGBA{c, pal.All[(i+1)%len(pal.All)]})
	}
	swapOf(all...)
	// The wall/floor tables are never touched by a swap.
	if chain.shade != plain.shade {
		t.Error("a sprite swap changed the wall shading")
	}
}

// A world's swaps don't leak into the next world, and setting a world is
// repeatable and the same on a fresh renderer.
func TestQASetWorldIsRepeatableAndLeaksNothing(t *testing.T) {
	var ice, crypt *proc.Theme
	for i := range proc.Themes {
		switch proc.Themes[i].Name {
		case "Ice Halls":
			ice = &proc.Themes[i]
		case "The Crypt":
			crypt = &proc.Themes[i]
		}
	}
	if ice == nil || len(ice.SpriteSwap) == 0 {
		t.Skip("no Ice Halls swap")
	}
	fresh := New(32, 20)
	fresh.SetWorld(crypt)
	used := New(32, 20)
	used.SetWorld(ice)
	used.SetWorld(crypt)
	if used.spriteShade != fresh.spriteShade || used.shade != fresh.shade || used.spriteLight != fresh.spriteLight {
		t.Error("Ice Halls left something behind in the Crypt")
	}
	again := New(32, 20)
	again.SetWorld(ice)
	again.SetWorld(ice)
	one := New(32, 20)
	one.SetWorld(ice)
	if again.spriteShade != one.spriteShade {
		t.Error("setting a world twice differs from once")
	}
}
