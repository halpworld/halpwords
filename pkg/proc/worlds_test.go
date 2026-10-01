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

// worldHash hashes everything a world draws at one seed: its walls, floor
// and its frames, ceiling, doors, torch, stairs, sky and prop, and its
// remixed walls.
func worldHash(th *Theme, seed uint64) string {
	ms := []*Indexed{th.WallTex(seed, 0), th.WallTex(seed, 1), th.WallTex(seed, 2),
		th.FloorTex(seed), th.CeilTex(seed), th.DoorTex(seed, false), th.DoorTex(seed, true),
		th.StairsTex(seed), th.Prop(seed)}
	ms = append(ms, th.FloorFrames(seed)...)
	for f := range Frames {
		ms = append(ms, th.TorchTex(seed, f))
	}
	if sky := th.SkyTex(seed); sky != nil {
		ms = append(ms, sky)
	}
	for lap := 1; lap <= 2; lap++ {
		r := th.Remix(lap)
		ms = append(ms, r.WallTex(seed, 0), r.WallTex(seed, 1), r.WallTex(seed, 2))
	}
	return indexedHash(ms...)
}

// Each world's textures at a fixed seed, so a change to how a world looks
// is always on purpose. Update a hash only when the new look is meant.
func TestWorldTexturesPinned(t *testing.T) {
	want := map[string]string{
		"The Crypt":          "a65d15bdeb62608658f63fe22c1a70156bae2d929174c1c4680594a393c40a0d",
		"Mossy Cellars":      "c26c6287c42315cd041dbb956f25247e6782c3454e7cd215702f202885c2156b",
		"Flooded Caves":      "36fc164a2554538bb1c6a72083e2649ada1c886cc05767ee8721637b652282b2",
		"Ice Halls":          "e9d8567d83659bca2298a99f45e094bc26ada6def2913de0634426dbd968beea",
		"Lava Forge":         "c31f9bb6c27a9c35da164bc9eddbd248829700f078321f3988e8912bc9de5690",
		"Amethyst Vaults":    "b1f6129d6c878456e61693568b22ae66cd5b4ca89e22d638c30a771777b445eb",
		"Clockwork Workshop": "e71c4e4d6aa4c8586cba57da4b761d4589c01122e27a1f51964f2d2588209750",
		"Sky Garden":         "69893c58e642807076f83828a4828aa9702c6f27d88d63b24de6c4ef5e01c6ba",
		"Sandstone Tomb":     "82f312a7dc1e0978a7b634c129a9ddfc8d4d72dbcf61503711f320edc8033b59",
		"Whispering Library": "2da9cfb946d95448db0c85408396b69be168c7840225f59acf5dc72d1bc6dfc1",
	}
	for i := range Themes {
		th := &Themes[i]
		if got := worldHash(th, 7); got != want[th.Name] {
			t.Errorf("%q: %q,", th.Name, got)
		}
	}
}

// meanLuminance is the average luminance of a texture's opaque pixels in
// the rectangle [x0,x1) × [y0,y1).
func meanLuminance(m *Indexed, x0, y0, x1, y1 int) float64 {
	sum, n := 0.0, 0
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if p := m.At(x, y); p != Transparent {
				sum += luminance(pal.All[p])
				n++
			}
		}
	}
	return sum / float64(n)
}

// contrast is the WCAG contrast ratio of two luminances.
func contrast(a, b float64) float64 {
	return (max(a, b) + 0.05) / (min(a, b) + 0.05)
}

// Doors, sealed doors and stairs stand out at least 3:1 from what is
// around them: a door's frame from the wall it is set in, and a stairwell's
// lit rim or its dark depths from the floor. The Crypt keeps today's
// textures, which the server also draws, so it is held only to today's
// numbers.
func TestDoorsAndStairsStandOut(t *testing.T) {
	const want, crypt = 3.0, 1.5
	for i := range Themes {
		for lap := range 3 {
			th := Themes[i].Remix(lap)
			need := want
			if th.Name == "The Crypt" {
				need = crypt
			}
			f := th.FrameRamp()
			for seed := range uint64(3) {
				wall := meanLuminance(th.WallTex(seed, 0), 0, 0, TexSize, TexSize)
				for _, sealed := range []bool{false, true} {
					door := th.DoorTex(seed, sealed)
					// The frame is the door texture's outer arch.
					frame := luminance(pal.All[door.At(4, TexSize-1)])
					if c := contrast(frame, wall); c < need {
						t.Errorf("%s lap %d seed %d: door frame (sealed %v) contrast %.2f, want %.1f", th.Name, lap, seed, sealed, c, need)
					}
				}
				floor := meanLuminance(th.FloorTex(seed), 0, 0, TexSize, TexSize)
				rim := contrast(luminance(f[len(f)-1]), floor)
				depths := contrast(luminance(f[0]), floor)
				if c := max(rim, depths); c < need {
					t.Errorf("%s lap %d seed %d: stairs contrast %.2f, want %.1f", th.Name, lap, seed, c, need)
				}
			}
		}
	}
}

// The runes on a sealed door glow, so it reads as sealed in the dark.
func TestSealedDoorsGlow(t *testing.T) {
	for i := range Themes {
		th := &Themes[i]
		open, sealed := th.DoorTex(1, false), th.DoorTex(1, true)
		if indexedHash(open) == indexedHash(sealed) {
			t.Errorf("%s: sealed door looks open", th.Name)
		}
		glow := 0
		for j, g := range sealed.Glow {
			if g && (open.Glow == nil || !open.Glow[j]) {
				glow++
			}
		}
		if glow < 40 {
			t.Errorf("%s: sealed door has %d glowing rune pixels", th.Name, glow)
		}
	}
}

// World gives the same worlds as ThemeFor, without allocating.
func TestWorld(t *testing.T) {
	for depth := 1; depth <= 40; depth++ {
		if World(FloorOrder[(depth-1)%len(FloorOrder)], Lap(depth)) != ThemeFor(depth) {
			t.Errorf("World and ThemeFor differ at depth %d", depth)
		}
	}
	if World(3, 0) != &Themes[3] || World(3, 1) == &Themes[3] || World(3, 1) != World(3, 3) {
		t.Error("World should be the world itself on lap 0 and a shared remix after")
	}
	if n := testing.AllocsPerRun(10, func() { World(4, 2) }); n != 0 {
		t.Errorf("World allocates %v times", n)
	}
}
