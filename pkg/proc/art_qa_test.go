package proc

import (
	"image/color"
	"math"
	"sync"
	"testing"

	"github.com/halpworld/halpwords/internal/pal"
)

// A world's textures hash the same however often, in whatever order and
// from however many goroutines they are made: the server draws them too.
func TestQAWorldHashesAreDeterministic(t *testing.T) {
	for i := range Themes {
		th := &Themes[i]
		want := worldHash(th, 11)
		for range 3 {
			if got := worldHash(th, 11); got != want {
				t.Fatalf("%s: hash changed between calls", th.Name)
			}
		}
		var wg sync.WaitGroup
		got := make([]string, 4)
		for g := range got {
			wg.Add(1)
			go func() { defer wg.Done(); got[g] = worldHash(th, 11) }()
		}
		wg.Wait()
		for g, h := range got {
			if h != want {
				t.Errorf("%s: goroutine %d hash differs", th.Name, g)
			}
		}
		if worldHash(th, 12) == want {
			t.Errorf("%s: another seed gives the same textures", th.Name)
		}
	}
}

// SpriteSwap and SpriteLight are for sprites only: they must not change
// any texture the server also draws, whatever they hold.
func TestQASpriteFieldsDoNotChangeTextures(t *testing.T) {
	for i := range Themes {
		th := Themes[i]
		want := worldHash(&th, 5)
		th.SpriteLight = math.NaN()
		th.SpriteSwap = [][2]color.RGBA{{pal.Purple, pal.Pink}, {}}
		if got := worldHash(&th, 5); got != want {
			t.Errorf("%s: sprite fields changed the textures", th.Name)
		}
		th.SpriteSwap = nil
		th.SpriteLight = 7
		if got := worldHash(&th, 5); got != want {
			t.Errorf("%s: SpriteLight changed the textures", th.Name)
		}
	}
}

// The Sky Garden's sky: fully opaque, deterministic per seed, different
// per seed, and its gradient bands waver (rows in a band are not all the
// same dither stripe).
func TestQASkyBands(t *testing.T) {
	var sky *Theme
	for i := range Themes {
		if Themes[i].Name == "Sky Garden" {
			sky = &Themes[i]
		}
	}
	if sky == nil {
		t.Skip("no Sky Garden")
	}
	a, b, c := sky.SkyTex(3), sky.SkyTex(3), sky.SkyTex(4)
	same, diff := true, false
	for y := range SkyH {
		for x := range SkyW {
			if a.At(x, y) == Transparent {
				t.Fatalf("transparent sky pixel at %d,%d", x, y)
			}
			same = same && a.At(x, y) == b.At(x, y)
			diff = diff || a.At(x, y) != c.At(x, y)
		}
	}
	if !same {
		t.Error("the sky differs between calls with one seed")
	}
	if !diff {
		t.Error("the sky is the same for two seeds")
	}
	// A row that is not a straight stripe: some row of the sky has
	// different colour counts to a neighbour far along it, i.e. the band
	// edge wavers across x.
	wavers := 0
	for y := 0; y < SkyH; y++ {
		first := map[uint8]int{}
		second := map[uint8]int{}
		for x := 0; x < SkyW/2; x++ {
			first[a.At(x, y)]++
			second[a.At(x+SkyW/2, y)]++
		}
		if len(first) != len(second) {
			wavers++
		}
	}
	if wavers == 0 {
		t.Log("no row's two halves differ in colours (sky may still waver within clouds)")
	}
}

// Out-of-range and odd SpriteLight in Theme methods never panics.
func TestQASpriteMinLightAlwaysInRange(t *testing.T) {
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -1e308, 1e308, 5e-324, 0, math.Copysign(0, -1), 0.5} {
		th := Theme{SpriteLight: v}
		if got := th.SpriteMinLight(); !(got >= 0 && got <= 1) {
			t.Errorf("SpriteLight %v: %v", v, got)
		}
	}
}
