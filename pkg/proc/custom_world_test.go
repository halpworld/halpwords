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

// families lists every exported family, modifier and animation, with the
// animations at frame 2.
var families = map[string]proc.Painter{
	"WallBricks": proc.WallBricks, "WallCave": proc.WallCave, "WallIce": proc.WallIce,
	"WallBasalt": proc.WallBasalt, "WallMetal": proc.WallMetal, "WallCrystal": proc.WallCrystal,
	"WallEarth": proc.WallEarth, "WallSandstone": proc.WallSandstone, "WallBooks": proc.WallBooks,
	"WallHedge":       proc.WallHedge,
	"FloorFlagstones": proc.FloorFlagstones, "FloorCobbles": proc.FloorCobbles, "FloorSand": proc.FloorSand,
	"FloorWater": proc.FloorWater, "FloorLava": proc.FloorLava, "FloorSnow": proc.FloorSnow,
	"FloorGrass": proc.FloorGrass, "FloorPlanks": proc.FloorPlanks, "FloorChecker": proc.FloorChecker,
	"FloorGrate": proc.FloorGrate,
	"CeilBeams":  proc.CeilBeams, "CeilRock": proc.CeilRock, "CeilIce": proc.CeilIce,
	"CeilPipes": proc.CeilPipes, "CeilCrystal": proc.CeilCrystal, "CeilSlabs": proc.CeilSlabs,
	"CeilCoffers": proc.CeilCoffers,
	"ModMoss":     proc.ModMoss, "ModCrack": proc.ModCrack, "ModCobweb": proc.ModCobweb,
	"ModFrost": proc.ModFrost, "ModDrips": proc.ModDrips, "ModVeins": proc.ModVeins,
	"ModGear": proc.ModGear, "ModPipe": proc.ModPipe, "ModFlowers": proc.ModFlowers,
	"ModRaggedTop": proc.ModRaggedTop, "ModVines": proc.ModVines, "ModSoot": proc.ModSoot,
	"ModGlowCracks": proc.ModGlowCracks, "ModSandDrift": proc.ModSandDrift,
	"AnimRipples": func(m *proc.Indexed, l proc.Look, seed uint64) { proc.AnimRipples(m, l, seed, 2) },
	"AnimLava":    func(m *proc.Indexed, l proc.Look, seed uint64) { proc.AnimLava(m, l, seed, 2) },
}

// No exported family panics on a short or empty ramp: Base of 0, 1 or 2
// colours with no Accent, and the other way round.
func TestFamiliesTakeShortRamps(t *testing.T) {
	ramp := []color.RGBA{{40, 30, 50, 255}, {120, 90, 60, 255}, {200, 180, 150, 255}}
	if len(families) != 43 {
		t.Fatalf("%d families listed, want 43", len(families))
	}
	for name, f := range families {
		for n := range 3 {
			for _, l := range []proc.Look{{Base: ramp[:n]}, {Accent: ramp[:n]}, {Base: ramp[:n], Accent: ramp[:1]}} {
				for seed := range uint64(40) {
					func() {
						defer func() {
							if r := recover(); r != nil {
								t.Fatalf("%s with %d Base and %d Accent colours, seed %d: %v", name, len(l.Base), len(l.Accent), seed, r)
							}
						}()
						f(proc.NewIndexed(proc.TexSize, proc.TexSize), l, seed)
					}()
				}
			}
		}
	}
}
