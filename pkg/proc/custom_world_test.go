package proc_test

import (
	"image/color"
	"slices"
	"testing"

	"github.com/halpworld/halpwords/pkg/proc"
)

// Another package can build a world of its own from the exported
// families, modifiers and animations, and draw every texture of it.
func TestWorldFromAnotherPackage(t *testing.T) {
	stone := []color.RGBA{{20, 20, 30, 255}, {60, 60, 70, 255}, {110, 110, 120, 255}, {170, 170, 180, 255}}
	glow := []color.RGBA{{200, 80, 40, 255}, {250, 200, 60, 255}}
	var mine proc.Modifier = func(m *proc.Indexed, l proc.Look, seed uint64) {
		m.Set(int(seed%proc.TexSize), 0, l.Accent[0])
	}
	th := proc.Theme{
		Name:     "Test Vault",
		Wall:     stone,
		Floor:    stone,
		Ceil:     stone,
		Moss:     glow,
		Walls:    proc.Surface{Paint: proc.WallBasalt, Mods: []proc.Modifier{proc.ModCrack}},
		Variants: [2][]proc.Modifier{{proc.ModGlowCracks}, {mine, proc.ModSoot}},
		Floors:   proc.Surface{Paint: proc.FloorLava, Anim: proc.AnimLava, Look: proc.Look{Base: stone, Accent: glow}},
		Ceiling:  proc.Surface{Paint: proc.CeilRock},
	}
	var p proc.Painter = proc.WallHedge
	if p == nil {
		t.Fatal("families are not Painters")
	}
	for v := range 3 {
		if th.WallTex(1, v) == nil {
			t.Fatalf("wall variant %d", v)
		}
	}
	if len(th.FloorFrames(1)) != proc.Frames || th.CeilTex(1) == nil || th.DoorTex(1, true) == nil || th.StairsTex(1) == nil {
		t.Fatal("a texture is missing")
	}
	if a, b := th.WallTex(1, 0), th.WallTex(1, 2); slices.Equal(a.Pix, b.Pix) {
		t.Error("the variant modifiers changed nothing")
	}
	if r := th.Remix(1); len(r.Walls.Mods) != 2 {
		t.Errorf("remix wall has %d modifiers, want 2", len(r.Walls.Mods))
	}
}

// A Theme with only the four ramps, as written before the worlds, draws
// with The Crypt's families: its textures equal The Crypt's.
func TestRampsOnlyThemeDraws(t *testing.T) {
	c := &proc.Themes[0]
	old := proc.Theme{Name: "Old", Wall: c.Wall, Floor: c.Floor, Ceil: c.Ceil, Moss: c.Moss}
	same := func(name string, a, b *proc.Indexed) {
		if !slices.Equal(a.Pix, b.Pix) {
			t.Errorf("%s differs from The Crypt's", name)
		}
	}
	same("wall", old.WallTex(7, 0), c.WallTex(7, 0))
	same("floor", old.FloorTex(7), c.FloorTex(7))
	same("ceiling", old.CeilTex(7), c.CeilTex(7))
	same("door", old.DoorTex(7, true), c.DoorTex(7, true))
	same("stairs", old.StairsTex(7), c.StairsTex(7))
}

// Stairs draw from a short ramp too: StairsTexture used to index past
// the start of a ramp under six colours, such as the Sky Garden's walls.
func TestStairsShortRamp(t *testing.T) {
	for i := range proc.Themes {
		th := &proc.Themes[i]
		if proc.StairsTexture(th, 1) == nil {
			t.Errorf("%s: no stairs", th.Name)
		}
	}
}
