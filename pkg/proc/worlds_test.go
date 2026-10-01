package proc

import (
	"image/color"
	"math"
	"slices"
	"testing"

	"github.com/halpworld/halpwords/internal/pal"
)

func TestFloorOrder(t *testing.T) {
	if got := ThemeFor(1).Name; got != "The Crypt" {
		t.Errorf("ThemeFor(1) = %s, want The Crypt", got)
	}
	want := []string{
		"The Crypt", "Mossy Cellars", "Flooded Caves", "Lava Forge", "Ice Halls",
		"Whispering Library", "Sky Garden", "Clockwork Workshop", "Amethyst Vaults", "Sandstone Tomb",
	}
	for i, name := range want {
		if got := ThemeFor(i + 1).Name; got != name {
			t.Errorf("ThemeFor(%d) = %s, want %s", i+1, got, name)
		}
		if got := ThemeFor(i + 11).Name; got != name {
			t.Errorf("ThemeFor(%d) = %s, want %s on the second lap", i+11, got, name)
		}
	}
	// Every world is in the order exactly once.
	order := slices.Sorted(slices.Values(FloorOrder))
	for i, w := range order {
		if w != i {
			t.Fatalf("FloorOrder %v is not a permutation of the %d worlds", FloorOrder, len(Themes))
		}
	}
	if len(FloorOrder) != len(Themes) {
		t.Fatalf("FloorOrder has %d worlds, Themes %d", len(FloorOrder), len(Themes))
	}
	// The bright Sky Garden never follows the Ice Halls.
	for i := range FloorOrder {
		if Themes[FloorOrder[i]].Name == "Ice Halls" && Themes[FloorOrder[(i+1)%len(FloorOrder)]].Name == "Sky Garden" {
			t.Error("Sky Garden follows the Ice Halls")
		}
	}
	if ThemeFor(0) != ThemeFor(1) || ThemeFor(-3) != ThemeFor(1) {
		t.Error("depths below 1 should get floor 1's world")
	}
}

func TestLap(t *testing.T) {
	for depth, want := range map[int]int{-1: 0, 0: 0, 1: 0, 10: 0, 11: 1, 20: 1, 21: 2, 31: 3} {
		if got := Lap(depth); got != want {
			t.Errorf("Lap(%d) = %d, want %d", depth, got, want)
		}
	}
	if ThemeFor(1) != &Themes[0] {
		t.Error("the first lap should use Themes itself")
	}
	if ThemeFor(11) == &Themes[0] || ThemeFor(11) != ThemeFor(31) {
		t.Error("laps 2 and 4 should share a remix that is not the first lap's world")
	}
}

// A remix changes how a world looks, but never which world it is.
func TestRemix(t *testing.T) {
	for i := range Themes {
		th := &Themes[i]
		if th.Remix(0) != th {
			t.Errorf("%s: Remix(0) is not the world itself", th.Name)
		}
		for lap := 1; lap <= 2; lap++ {
			r := th.Remix(lap)
			if r.Name != th.Name || !slices.Equal(r.Wall, th.Wall) {
				t.Errorf("%s lap %d: the name or ramps changed", th.Name, lap)
			}
			if r.Fog == th.Fog && th.Fog != th.LightTint() {
				t.Errorf("%s lap %d: the fog did not shift", th.Name, lap)
			}
			if r.PropRooms <= th.PropRooms {
				t.Errorf("%s lap %d: props are not denser", th.Name, lap)
			}
			if indexedHash(r.WallTex(5, 0)) == indexedHash(th.WallTex(5, 0)) {
				t.Errorf("%s lap %d: the plain wall did not change", th.Name, lap)
			}
		}
		// Remixing must not touch the original's modifier lists.
		before := len(th.Walls.Mods)
		th.Remix(1)
		th.Remix(2)
		if len(th.Walls.Mods) != before {
			t.Errorf("%s: Remix changed the original", th.Name)
		}
	}
}

// The Crypt's textures in the 3D view are exactly the old ones.
func TestCryptLooksAsBefore(t *testing.T) {
	th := &Themes[0]
	for seed := range uint64(4) {
		for v := range 3 {
			if indexedHash(th.WallTex(seed, v)) != indexedHash(WallTexture(th, seed, v)) {
				t.Errorf("seed %d wall %d differs", seed, v)
			}
		}
		for _, sealed := range []bool{false, true} {
			if indexedHash(th.DoorTex(seed, sealed)) != indexedHash(DoorTexture(th, seed, sealed)) {
				t.Errorf("seed %d door (sealed %v) differs", seed, sealed)
			}
		}
		for f := range 4 {
			if indexedHash(th.TorchTex(seed, f)) != indexedHash(TorchWall(th, seed, f)) {
				t.Errorf("seed %d torch frame %d differs", seed, f)
			}
		}
		if indexedHash(th.FloorTex(seed)) != indexedHash(FloorTexture(th, seed)) {
			t.Errorf("seed %d floor differs", seed)
		}
		if indexedHash(th.StairsTex(seed)) != indexedHash(StairsTexture(th, seed)) {
			t.Errorf("seed %d stairs differ", seed)
		}
		if indexedHash(th.CeilTex(seed)) != indexedHash(CeilingTexture(th, seed)) {
			t.Errorf("seed %d ceiling differs", seed)
		}
	}
	if th.FloorFrames(1) != nil || th.SkyTex(1) != nil {
		t.Error("The Crypt has no animated floor or sky")
	}
}

// Every texture tiles: the families are painted for a 32×32 repeat and
// leave no transparent holes, except where a world means to show the sky.
func TestWorldTexturesAreSolid(t *testing.T) {
	for i := range Themes {
		th := &Themes[i]
		check := func(what string, m *Indexed, holesOK bool) {
			t.Helper()
			if m.W != TexSize || m.H != TexSize {
				t.Errorf("%s %s: %dx%d", th.Name, what, m.W, m.H)
			}
			if !holesOK && slices.Contains(m.Pix, Transparent) {
				t.Errorf("%s %s has transparent pixels", th.Name, what)
			}
		}
		see := len(th.Sky) > 0 // hedge tops show the sky through them
		for v := range 3 {
			check("wall", th.WallTex(1, v), see)
		}
		check("floor", th.FloorTex(1), false)
		check("ceiling", th.CeilTex(1), false)
		check("stairs", th.StairsTex(1), false)
		check("door", th.DoorTex(1, false), see)
		for _, f := range th.FloorFrames(1) {
			check("floor frame", f, false)
		}
		if p := th.Prop(1); p.W != PropSizePx || p.H != PropSizePx {
			t.Errorf("%s prop is %dx%d", th.Name, p.W, p.H)
		}
		if th.PropSize <= 0 || th.PropSize > 0.4 {
			t.Errorf("%s prop size %v, want (0, 0.4]", th.Name, th.PropSize)
		}
	}
}

// Animated floors loop in four frames that really differ.
func TestFloorFrames(t *testing.T) {
	animated := 0
	for i := range Themes {
		fs := Themes[i].FloorFrames(3)
		if fs == nil {
			continue
		}
		animated++
		if len(fs) != Frames {
			t.Fatalf("%s: %d frames", Themes[i].Name, len(fs))
		}
		if indexedHash(fs[0]) != indexedHash(Themes[i].FloorTex(3)) {
			t.Errorf("%s: frame 0 is not the still floor", Themes[i].Name)
		}
		for f := 1; f < Frames; f++ {
			if indexedHash(fs[f]) == indexedHash(fs[f-1]) {
				t.Errorf("%s: frames %d and %d are the same", Themes[i].Name, f-1, f)
			}
		}
	}
	if animated != 2 {
		t.Errorf("%d animated floors, want water and lava", animated)
	}
}

// luminance is the relative luminance of a colour (WCAG).
func luminance(c color.RGBA) float64 {
	lin := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

// The open sky must never glare: no fog or sky colour above about 85%
// luminance, clouds included.
func TestSkyIsNotTooBright(t *testing.T) {
	for i := range Themes {
		th := &Themes[i]
		if len(th.Sky) == 0 {
			continue
		}
		for _, fog := range []color.RGBA{th.Fog, th.Remix(1).Fog} {
			if l := luminance(fog); l > 0.85 {
				t.Errorf("%s fog luminance %.2f", th.Name, l)
			}
		}
		sky := th.SkyTex(1)
		if sky.W != SkyW || sky.H != SkyH {
			t.Fatalf("sky is %dx%d", sky.W, sky.H)
		}
		seen := map[uint8]bool{}
		for _, p := range sky.Pix {
			seen[p] = true
		}
		for p := range seen {
			if p == Transparent {
				t.Fatalf("%s sky has holes", th.Name)
			}
			if l := luminance(pal.All[p]); l > 0.85 {
				t.Errorf("%s sky colour %d has luminance %.2f", th.Name, p, l)
			}
		}
	}
}
