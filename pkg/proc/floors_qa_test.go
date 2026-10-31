package proc

import (
	"slices"
	"testing"
)

// mainFloors are floors 1 to 10 as origin/main drew them (the world's name,
// and worldHash at seed 1234+depth), taken before the Sunken Temple and
// Candy Caverns were added: the new worlds join at the end of the order,
// so nothing a player has seen changes.
var mainFloors = []struct {
	depth int
	name  string
	hash  string
}{
	{1, "The Crypt", "5d9d0656e4914f740509971c4828554b8ee412f7c06068854594cdac408cd55c"},
	{2, "Mossy Cellars", "58655962a39c343f7c1dd4a8c2a7e4c1a4a497b236e514d217fe45dc3adf8e38"},
	{3, "Flooded Caves", "b724a235f7bf7a257695fe145955de9068ae3d7232dbc613f7b3b9c295a51c6e"},
	{4, "Lava Forge", "61281a7e927186672f14e75a89afbccec412757989914511de05e00415475209"},
	{5, "Ice Halls", "b7be3a9c2751c71e679b45693ca9289a41c3eb83d00af25caf6bcd5430213e33"},
	{6, "Whispering Library", "cccbcb45d829fd32aee49aa51b4515cbd9fccc29ffeec3995c858dfbe95df1f6"},
	{7, "Sky Garden", "a2cf7ad39ef7564ef03fc6d5cf53a87c95abb051de76aeb1e00a0d2889060caa"},
	{8, "Clockwork Workshop", "c6f9dcfc32193e262733e3d2a667089f75cd77d8b4c26ed2ce192904103fad87"},
	{9, "Amethyst Vaults", "22cfea4538c26bc0998c1b89b89d621b118f0cfdb60e383e181026fa2ba092cc"},
	{10, "Sandstone Tomb", "3f731c16a1034acc4a4b96537128c6009ee272b59f848b6113fdf29ce52e3c58"},
}

func TestQAFloorsOneToTenAreAsOnMain(t *testing.T) {
	for _, f := range mainFloors {
		th := ThemeFor(f.depth)
		if th.Name != f.name {
			t.Errorf("floor %d is %q, was %q", f.depth, th.Name, f.name)
		}
		if got := worldHash(th, 1234+uint64(f.depth)); got != f.hash {
			t.Errorf("floor %d (%s): textures changed from main: %s", f.depth, th.Name, got)
		}
	}
}

// Every floor from 1 to 60 has a world, the new ones are floors 11 and 12,
// each lap repeats the order, later laps are remixes that differ from the
// first, laps 3 and up repeat lap 2's look, and building the same floor
// twice (also from several goroutines) gives the same textures.
func TestQAEveryFloorHasAWorldAndIsDeterministic(t *testing.T) {
	if n := len(FloorOrder); n != len(Themes) {
		t.Fatalf("%d floors in an order, %d worlds", n, len(Themes))
	}
	if !slices.Equal(FloorOrder[:10], []int{0, 1, 2, 4, 3, 9, 7, 6, 5, 8}) {
		t.Errorf("the first ten floors moved: %v", FloorOrder[:10])
	}
	if ThemeFor(11).Name != "Sunken Temple" || ThemeFor(12).Name != "Candy Caverns" {
		t.Errorf("floors 11 and 12 are %q and %q", ThemeFor(11).Name, ThemeFor(12).Name)
	}
	seen := map[string]bool{}
	for d := 1; d <= len(FloorOrder); d++ {
		seen[ThemeFor(d).Name] = true
	}
	if len(seen) != len(Themes) {
		t.Errorf("the first lap has %d different worlds of %d", len(seen), len(Themes))
	}
	n := len(FloorOrder)
	for d := 1; d <= 60; d++ {
		th := ThemeFor(d)
		if th == nil || th.Name == "" {
			t.Fatalf("floor %d has no world", d)
		}
		lap := Lap(d)
		if want := (d - 1) / n; lap != want {
			t.Errorf("floor %d: lap %d, want %d", d, lap, want)
		}
		if d > n && th.Name != ThemeFor(d-n).Name {
			t.Errorf("floor %d is %q, floor %d is %q", d, th.Name, d-n, ThemeFor(d-n).Name)
		}
		if lap >= 1 && worldHash(th, 3) == worldHash(&Themes[FloorOrder[(d-1)%n]], 3) {
			t.Errorf("floor %d looks exactly like its world's first lap", d)
		}
		if lap >= 3 && worldHash(th, 3) != worldHash(ThemeFor(d-2*n), 3) {
			t.Errorf("floor %d: laps repeat from the third on, but this one differs from floor %d", d, d-2*n)
		}
		a, b := worldHash(th, uint64(d)), worldHash(ThemeFor(d), uint64(d))
		if a != b {
			t.Errorf("floor %d is not deterministic", d)
		}
	}
	// Lap 0 of an out-of-range or negative depth doesn't panic.
	for _, d := range []int{0, -1, -100, 1 << 30} {
		if ThemeFor(d) == nil {
			t.Errorf("depth %d has no world", d)
		}
	}
}
