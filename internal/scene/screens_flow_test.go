package scene

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/pkg/words"
)

// Through Update, as the game loop runs it: Esc on the quest intro goes to
// the hero picker (#47).
func TestQuestIntroEscThroughUpdate(t *testing.T) {
	ctx := testContext(t)
	fr, _ := words.Lookup("fr")
	q := builtInQuests()[0].q
	setup := runSetup{quest: q}
	r := newRun(ctx, fr, rpg.Knight, setup)
	page := questIntro(r, setup)
	next := ctx.TestScenes(page)

	holdKey(t, ebiten.KeyEscape)
	if err := page.Update(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := next().(*ClassPick); !ok {
		t.Error("Esc on the quest intro should lead to the hero picker")
	}
}

// The Rankings tab: C opens the code box, typing fills it, Esc closes it
// without leaving the screen (#47).
func TestRankingsCheckBox(t *testing.T) {
	ctx := rankContext(t)
	withFont(t, ctx)
	ctx.Input = &input.State{}
	h := NewHallOfFame(ctx).(*HallOfFame)
	next := ctx.TestScenes(h)
	h.mi = len(fameModes)
	if !h.ranking() {
		t.Fatal("not on the Rankings tab")
	}

	holdKey(t, ebiten.KeyC)
	h.Update(ctx)
	if !h.checking {
		t.Fatal("C did not open the box on Rankings")
	}

	// The box draws (pixels can't be read outside a running game, so the
	// draw-the-box fix itself is checked by the code path: Draw on the
	// Rankings tab calls drawCheck), and the hint is the box's.
	h.Draw(ebiten.NewImage(game.ScreenW, game.ScreenH), ctx)
	if got := h.help(ctx); got != "Type the share code · Enter check · Esc close" {
		t.Errorf("hint %q", got)
	}

	// Typing goes into the box, not the tab.
	ctx.Input.Chars = []rune("hw-fr")
	h.Update(ctx)
	if string(h.code) != "HW-FR" {
		t.Errorf("code %q", string(h.code))
	}
	if h.mi != len(fameModes) {
		t.Error("typing changed the tab")
	}

	// Esc closes the box and stays on the screen.
	holdKey(t, ebiten.KeyEscape)
	h.Update(ctx)
	if h.checking {
		t.Error("Esc did not close the box")
	}
	if next() != nil {
		t.Error("Esc with the box open left the screen")
	}
	if !h.ranking() {
		t.Error("left the Rankings tab")
	}
}
