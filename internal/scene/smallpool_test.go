package scene

import (
	"fmt"
	"strings"
	"testing"

	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/pkg/compete"
	"github.com/halpworld/halpwords/pkg/puzzle"
	"github.com/halpworld/halpwords/pkg/words"
)

// A run on 1 to 8 ticked words (#89) deals fights and every kind of
// puzzle on every floor without failing, and never the same word twice in
// a row when it has two or more.
func TestSmallSelectionsDeal(t *testing.T) {
	ctx := testContext(t)
	fr, _ := words.Lookup("fr")
	lines := strings.Split(strings.TrimSpace(eight), "\n")[3:] // the word lines
	for n := 1; n <= len(lines); n++ {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			ctx := *ctx
			src := "title: Small\nlanguage: fr\n\n" + strings.Join(lines[:n], "\n") + "\n"
			ctx.Lists = append(ctx.Lists[:len(ctx.Lists):len(ctx.Lists)], ownList(t, "small.txt", src))
			setup := runSetup{mode: compete.Adventure, pool: listPool{keys: []string{"file:small.txt"}}}
			r := newRun(&ctx, fr, rpg.Rogue, setup)
			if r.deck.Len() != n {
				t.Fatalf("%d words in the deck", r.deck.Len())
			}
			for depth := 1; depth <= 30; depth++ {
				last := -1 // a puzzle deals words between the floors' fights
				if l := r.floor(depth); l == nil {
					t.Fatalf("no floor %d", depth)
				}
				for range 8 {
					e, id, ok := r.deck.NextNear(float64(depth)/30, nil)
					if !ok || e.Prompt == "" {
						t.Fatalf("floor %d: no word", depth)
					}
					if n > 1 && id == last {
						t.Fatalf("floor %d: %q twice in a row", depth, e.Prompt)
					}
					last = id
					r.deck.Mark(id, depth%3 != 0)
				}
				for _, lock := range []puzzle.Lock{puzzle.Door, puzzle.Chest} {
					for _, k := range puzzle.Kinds(lock, depth) {
						if p := puzzle.Make(k, lock, depth, r.deck, r.lang, r.rules(), r.rng); p == nil || p.Ask() == "" {
							t.Fatalf("floor %d: no %v", depth, k)
						}
					}
					if p := puzzle.New(lock, depth, r.deck, r.lang, r.rules(), r.rng); p == nil {
						t.Fatalf("floor %d: no puzzle", depth)
					}
				}
			}
		})
	}
}
