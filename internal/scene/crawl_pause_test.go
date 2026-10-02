package scene

import (
	"testing"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/pkg/words"
)

func testCrawl(t *testing.T, ctx *game.Context) *Crawl {
	t.Helper()
	lang, _ := words.Lookup("fr")
	r := startRun(ctx, lang, rpg.Knight, 5, nil)
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
	if c.canChoose(pauseFlee) || c.canChoose(pauseItems) || c.canChoose(pauseSuspend) {
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
	if !c.canChoose(pauseFlee) || !c.canChoose(pauseItems) || c.canChoose(pauseSuspend) {
		t.Fatal("on the hero's turn they should be able to flee and use items, but not suspend")
	}
}

func TestUnsavedProgress(t *testing.T) {
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	if !c.hasUnsaved() {
		t.Fatal("a new adventure counts as saved")
	}
	c.lastSave, _ = encodeSave(c.run, c.level, c.pos, c.facing, true)
	c.pause(ctx)
	if c.unsaved || !c.canChoose(pauseSuspend) {
		t.Fatalf("unsaved %v right after saving", c.unsaved)
	}
	c.unpause(ctx)
	c.facing = c.facing.Right()
	if !c.hasUnsaved() {
		t.Fatal("turning around did not count as progress")
	}
}

// Time in the pause menu and other menus is not played time, and the
// pause menu's "unsaved" check does not count it, so quitting right after
// a save does not warn.
func TestPausedTimeIsNotPlayedTime(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	c.pray(ctx)
	before := c.run.played
	c.pause(ctx)
	for i := 0; i < 120; i++ {
		if err := c.Update(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if c.run.played != before {
		t.Fatalf("paused for 120 ticks: played %v -> %v", before, c.run.played)
	}
	for _, m := range []mode{modeMap, modeQuit, modeShop, modeItems, modeCampfire, modeShrine, modeDead} {
		c.mode = m
		c.countPlayed()
		if c.run.played != before {
			t.Fatalf("mode %d counted as played time", m)
		}
	}
	for _, m := range []mode{modeExplore, modeBattle, modePuzzle} {
		c.run.played = before
		c.mode = m
		c.countPlayed()
		if c.run.played <= before {
			t.Fatalf("mode %d did not count as played time", m)
		}
	}
	// Played time alone is not an unsaved change.
	c.mode = modeExplore
	c.run.played += 30
	if c.hasUnsaved() {
		t.Fatal("played time made a saved game unsaved")
	}
	c.run.hero.Gold++
	if !c.hasUnsaved() {
		t.Fatal("a change was missed")
	}
}
