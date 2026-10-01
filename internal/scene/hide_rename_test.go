package scene

import (
	"testing"

	"github.com/halpworld/halpwords/pkg/compete"
)

// If the provisional entry is gone from the table when the name is
// entered, the run is added again, not lost.
func TestProvisionalEntryGoneIsAddedAgain(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	c := hardcoreCrawl(t, ctx, 3)
	c.die()
	g := newGameOver(ctx, c.run, false)
	key := compete.TableKey(compete.Hardcore, "fr")
	g.name = []rune("Ad")
	g.OnHide(ctx)
	ctx.Profile.Fame.Table(key)[0].Score = g.score + 1 // no longer matches
	g.name = []rune("Ada")
	g.record(ctx)
	found := false
	for _, f := range ctx.Profile.Fame.Table(key) {
		if f.Name == "Ada" && f.Score == g.score {
			found = true
		}
	}
	if !found {
		t.Fatalf("the run was lost: %+v", ctx.Profile.Fame.Table(key))
	}
}
