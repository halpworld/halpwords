package scene

import (
	"testing"

	"github.com/halpworld/halpwords/pkg/maps"
)

// A quest map that names no theme looks as such maps always did: two
// levels per theme, through the six map themes.
func TestQuestThemeIsTheOldOne(t *testing.T) {
	for level, want := range map[int]string{
		0: "The Crypt", 1: "The Crypt", 2: "The Crypt", 3: "Mossy Cellars", 5: "Flooded Caves",
		12: maps.Themes[5], 13: "The Crypt", 25: maps.Themes[0],
	} {
		if got := questTheme(level).Name; got != want {
			t.Errorf("questTheme(%d) = %s, want %s", level, got, want)
		}
	}
	for level := 1; level <= 40; level++ {
		if got, want := questTheme(level).Name, maps.Themes[((level-1)/2)%6]; got != want {
			t.Errorf("questTheme(%d) = %s, want %s", level, got, want)
		}
	}
}
