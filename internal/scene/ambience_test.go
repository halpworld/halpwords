package scene

import (
	"testing"

	"github.com/halpworld/halpwords/internal/audio"
)

func TestEveryWorldHasAmbience(t *testing.T) {
	names := []string{"The Crypt", "Mossy Cellars", "Flooded Caves", "Lava Forge", "Ice Halls",
		"Whispering Library", "Sky Garden", "Clockwork Workshop", "Amethyst Vaults", "Sandstone Tomb"}
	if len(worldAmbience) != len(names) {
		t.Errorf("%d ambiences for %d worlds", len(worldAmbience), len(names))
	}
	seen := map[audio.Ambience]string{}
	for _, n := range names {
		a := worldAmbience[n]
		if a == audio.None {
			t.Errorf("%s has no ambience", n)
		}
		if o, ok := seen[a]; ok {
			t.Errorf("%s and %s share %s", n, o, a)
		}
		seen[a] = n
	}
}
