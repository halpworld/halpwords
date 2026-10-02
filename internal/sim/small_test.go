package sim

import (
	"fmt"
	"testing"

	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/pkg/words"
)

// A run on a handful of ticked words (#89) plays every floor: no stall,
// no panic, the same words coming round.
func TestSmallSelectionsPlay(t *testing.T) {
	all := []words.Entry{
		{Prompt: "red", Answers: []string{"rouge"}}, {Prompt: "blue", Answers: []string{"bleu"}},
		{Prompt: "green", Answers: []string{"vert"}}, {Prompt: "black", Answers: []string{"noir"}},
		{Prompt: "white", Answers: []string{"blanc"}}, {Prompt: "yellow", Answers: []string{"jaune"}},
		{Prompt: "pink", Answers: []string{"rose"}}, {Prompt: "grey", Answers: []string{"gris"}},
	}
	for n := 1; n <= len(all); n++ {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			for _, ty := range Typists {
				floors := Run(ty, rpg.Knight, all[:n], uint64(n), 6)
				if len(floors) != 6 {
					t.Fatalf("%s: %d floors", ty.Name, len(floors))
				}
				for _, f := range floors {
					if f.Fights == 0 {
						t.Errorf("%s: no fights on floor %d", ty.Name, f.Depth)
					}
				}
			}
		})
	}
}
