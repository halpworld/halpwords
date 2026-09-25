package scene

import (
	"slices"
	"strings"
	"testing"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/pkg/puzzle"
)

// dealKind puts a puzzle of kind k on a lock in front of the hero.
func dealKind(t *testing.T, c *Crawl, lock puzzle.Lock, k puzzle.Kind) *lockPuzzle {
	t.Helper()
	c.run.depth = 7
	c.startPuzzle(c.pos, lock)
	for n := 0; c.puzzle.p.Kind() != k; n++ {
		if n == 500 {
			t.Fatalf("no %s puzzle dealt", k)
		}
		c.dealPuzzle()
	}
	return c.puzzle
}

// Swapping meanings keeps one meaning on each row, and can match them all.
func TestSwapMeaning(t *testing.T) {
	c := testCrawl(t, testContext(t))
	lp := dealKind(t, c, puzzle.Door, puzzle.Pairs)
	n := len(lp.choice)
	for row := range lp.choice {
		lp.pick = row
		c.swapMeaning(lp, (row-lp.choice[row]+n)%n)
		if lp.choice[row] != row {
			t.Fatalf("row %d has meaning %d after swapping", row, lp.choice[row])
		}
		if s := slices.Sorted(slices.Values(lp.choice)); !slices.Equal(s, []int{0, 1, 2, 3}) {
			t.Fatalf("choice %v is not one meaning per row", lp.choice)
		}
	}
	c.solvePuzzle()
	if !lp.solved || c.level.At(c.pos) != dungeon.OpenDoor {
		t.Fatalf("matching every pair did not open the door: %q", lp.lines)
	}
}

// The wheels skip fixed plates, and turning them to the right letters
// opens the chest.
func TestTumblerWheels(t *testing.T) {
	c := testCrawl(t, testContext(t))
	c.level.Chests[c.pos] = &dungeon.Chest{Gold: 3}
	lp := dealKind(t, c, puzzle.Chest, puzzle.Tumbler)
	opts := lp.p.(puzzle.Chooser).Options()
	i := lp.pick
	for n := 0; n < 2*len(opts); n++ {
		if len(opts[i]) < 2 {
			t.Fatalf("wheel %d is a fixed plate %q", i, opts[i])
		}
		i = nextWheel(lp, i, 1)
	}
	want := lp.p.Check(puzzle.Attempt{}).Expected
	spelled := ""
	for i, o := range opts {
		for k, l := range o {
			if strings.HasPrefix(want, spelled+l) {
				lp.choice[i] = k
			}
		}
		spelled += o[lp.choice[i]]
	}
	if got := dialed(lp); got != want {
		t.Fatalf("wheels show %q, want %q", got, want)
	}
	c.solvePuzzle()
	if !lp.solved || !c.level.Chests[c.pos].Open {
		t.Fatalf("the right wheels did not open the chest: %q", lp.lines)
	}
}
