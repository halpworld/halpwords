package proc

import (
	"fmt"
	"testing"
)

// The first six themes keep their names and order for ever: saves, the
// server and old hashes refer to them by index (#72).
func TestQAThemesAppendOnly(t *testing.T) {
	want := []string{"The Crypt", "Mossy Cellars", "Flooded Caves", "Ice Halls", "Lava Forge", "Amethyst Vaults"}
	if len(Themes) < len(want) {
		t.Fatalf("only %d themes", len(Themes))
	}
	for i, n := range want {
		if Themes[i].Name != n {
			t.Errorf("Themes[%d] = %q, want %q", i, Themes[i].Name, n)
		}
	}
	seen := map[string]bool{}
	for _, th := range Themes {
		if seen[th.Name] {
			t.Errorf("duplicate theme %q", th.Name)
		}
		seen[th.Name] = true
	}
}

// Depths far outside the game's range do not panic, and every one has a
// world, laps cycle through the two remixes.
func TestQAHugeAndOddDepths(t *testing.T) {
	for _, d := range []int{-1 << 40, -1, 0, 1, 10, 11, 21, 31, 1000, 1 << 20, 1<<31 - 1, int(^uint(0) >> 1)} {
		th := ThemeFor(d)
		if th == nil || th.Name == "" {
			t.Fatalf("ThemeFor(%d) = %v", d, th)
		}
		_ = Lap(d)
		_ = th.WallTex(1, 2)
	}
	if ThemeFor(21) != ThemeFor(41) {
		t.Error("lap 2 and lap 4 should share a remix")
	}
	if ThemeFor(11) == ThemeFor(21) {
		t.Error("laps 1 and 2 should have different remixes")
	}
	if ThemeFor(11) != ThemeFor(31) {
		t.Error("laps 1 and 3 should share a remix")
	}
	// Floors 1-10 are the designed worlds, never remixes.
	for d := 1; d <= 10; d++ {
		if ThemeFor(d) != &Themes[FloorOrder[d-1]] {
			t.Errorf("floor %d is not the base world", d)
		}
	}
}

// Building textures for every world and lap must not modify the shared
// Themes or remixes (slices appended to in place would).
func TestQABuildingDoesNotMutateThemes(t *testing.T) {
	snap := func() string { return fmt.Sprintf("%#v|%#v", Themes, remixes) }
	before := snap()
	for d := 1; d <= 45; d++ {
		th := ThemeFor(d)
		for v := range 3 {
			th.WallTex(uint64(d), v)
		}
		th.FloorTex(1)
		th.FloorFrames(1)
		th.CeilTex(1)
		th.DoorTex(1, true)
		th.TorchTex(1, 2)
		th.StairsTex(1)
		th.SkyTex(1)
		if th.Prop != nil {
			th.Prop(uint64(d))
		}
	}
	if snap() != before {
		t.Error("building textures changed a theme")
	}
	// Remix is repeatable and does not alias the original's slices.
	a, b := Themes[0].Remix(1), Themes[0].Remix(1)
	h := indexedHash(b.WallTex(1, 0), b.WallTex(1, 1), b.WallTex(1, 2))
	for i := range a.Walls.Mods {
		a.Walls.Mods[i] = nil
	}
	for i := range a.Variants[1] {
		a.Variants[1][i] = nil
	}
	if indexedHash(b.WallTex(1, 0), b.WallTex(1, 1), b.WallTex(1, 2)) != h ||
		indexedHash(Themes[0].Remix(1).WallTex(1, 0), Themes[0].Remix(1).WallTex(1, 1), Themes[0].Remix(1).WallTex(1, 2)) != h {
		t.Error("remixes share a modifier slice")
	}
}

// Textures are a pure function of theme and seed.
func TestQATexturesRepeatable(t *testing.T) {
	for d := 1; d <= 25; d++ {
		th := ThemeFor(d)
		a := indexedHash(th.WallTex(3, 1), th.FloorTex(4), th.CeilTex(5), th.DoorTex(6, false), th.StairsTex(7))
		b := indexedHash(th.WallTex(3, 1), th.FloorTex(4), th.CeilTex(5), th.DoorTex(6, false), th.StairsTex(7))
		if a != b {
			t.Errorf("depth %d: textures differ between builds", d)
		}
	}
}
