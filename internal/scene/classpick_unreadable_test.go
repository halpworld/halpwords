package scene

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/pkg/compete"
	"github.com/halpworld/halpwords/pkg/words"
)

// A save that exists but can't be read is still a save a new run would
// replace, so the player must be asked.
func TestClassPickAsksBeforeReplacingAnUnreadableSave(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	ctx.Input = &input.State{}
	next := ctx.TestScenes(nil)
	arrange(t, ctx, unreadableSave)
	fr, _ := words.Lookup("fr")
	p := NewClassPick(fr, runSetup{mode: compete.Adventure}).(*ClassPick)
	press(t, ebiten.KeyEnter)
	p.Update(ctx)
	if !p.confirming() || next() != nil {
		t.Fatal("started a run over an unreadable save without asking")
	}
}
