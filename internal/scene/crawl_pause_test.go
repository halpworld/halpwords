package scene

import (
	"testing"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/words"
)

func testCrawl(t *testing.T, ctx *game.Context) *Crawl {
	t.Helper()
	lang, _ := words.Lookup("fr")
	r := startRun(ctx, lang, 5)
	r.sound = &game.Sound{Muted: true}
	l := dungeon.Generate(r.floorSeed(1), 1)
	return &Crawl{run: r, level: l, pos: l.Start, facing: l.StartDir}
}

// Time spent paused must not count against the hero in a battle.
func TestPauseStopsTheBattleClock(t *testing.T) {
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	c.mode = modeBattle
	c.battle = &battle{m: c.level.Monsters[0], phase: phaseDefend, start: 100, shown: 90}

	ctx.Tick = 130
	c.pause(ctx)
	if c.mode != modePause {
		t.Fatalf("mode %d, want paused", c.mode)
	}
	if c.canChoose(pauseFlee) || c.canChoose(pauseSave) || c.canChoose(pauseSaveQuit) {
		t.Fatal("can flee or save while dodging")
	}
	ctx.Tick += 600
	c.unpause(ctx)
	if c.mode != modeBattle {
		t.Fatalf("mode %d after resuming, want battle", c.mode)
	}
	if b := c.battle; b.start != 700 || b.shown != 690 {
		t.Fatalf("clock at start %d shown %d, want 700 and 690", b.start, b.shown)
	}

	c.battle.phase = phaseAttack
	c.pause(ctx)
	if !c.canChoose(pauseFlee) || c.canChoose(pauseSave) {
		t.Fatal("on the hero's turn they should be able to flee but not save")
	}
}

func TestUnsavedProgress(t *testing.T) {
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	if !c.hasUnsaved() {
		t.Fatal("a new adventure counts as saved")
	}
	c.lastSave, _ = encodeSave(c.run, c.level, c.pos, c.facing)
	c.pause(ctx)
	if c.unsaved || !c.canChoose(pauseSave) {
		t.Fatalf("unsaved %v right after saving", c.unsaved)
	}
	c.unpause(ctx)
	c.facing = c.facing.Right()
	if !c.hasUnsaved() {
		t.Fatal("turning around did not count as progress")
	}
}
