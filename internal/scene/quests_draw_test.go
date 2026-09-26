package scene

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/assets"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/unifont"
)

// The quest screens draw without failing, with a message and a long text.
func TestQuestScreensDraw(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	face, err := unifont.ParseBytes(assets.UnifontHex)
	if err != nil {
		t.Fatal(err)
	}
	ctx.Font = gfx.NewFont(face)
	dst := ebiten.NewImage(game.ScreenW, game.ScreenH)
	s := NewQuests(ctx, droppedFile{"bad.hwmap", []byte("{}")}).(*Quests)
	s.Draw(dst, ctx)
	c := questRun(t)
	questIntro(c.run).Draw(dst, ctx)
	questEnd(c.run).Draw(dst, ctx)
}
