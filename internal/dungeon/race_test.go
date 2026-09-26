package dungeon

import (
	"testing"

	"github.com/halpworld/halpwords/pkg/race"
)

// TestRaceRulesFitTheDungeon checks the numbers pkg/race judges racers
// by (halpwords-server leaves a racer whose reports break them out of the
// results) against generated floors: their size, the shortest walk to the
// stairs, and how many monsters a floor can hold.
func TestRaceRulesFitTheDungeon(t *testing.T) {
	for d := 1; d <= 40; d++ {
		if race.MapSize(d) != Size(d) {
			t.Fatalf("race.MapSize(%d) = %d, Size = %d", d, race.MapSize(d), Size(d))
		}
	}
	n := uint64(20000)
	if testing.Short() {
		n = 2000
	}
	for d := 1; d <= race.Goal; d++ {
		fewest := -1
		for s := range n {
			l := Generate(s*2654435761+12345, d)
			l.Harden() // races have Hardcore's rules
			if steps := l.distances(l.Start, true)[l.Index(l.Exit)]; fewest < 0 || steps < fewest {
				fewest = steps
			}
			if most := len(l.Monsters) + len(l.Chests); most > race.MaxMonsters(d) {
				t.Fatalf("floor %d (seed %d) can hold %d monsters, race.MaxMonsters = %d", d, s, most, race.MaxMonsters(d))
			}
		}
		if d < race.Goal && fewest < race.MinStairsSteps {
			t.Errorf("floor %d: stairs %d steps from the start, race.MinStairsSteps = %d", d, fewest, race.MinStairsSteps)
		}
	}
}
