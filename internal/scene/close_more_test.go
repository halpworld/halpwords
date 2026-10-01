package scene

import (
	"testing"
	"time"

	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/pkg/compete"
)

// Closing in a place where Suspend is not offered must leave an existing
// save exactly as it was.
func TestCloseKeepsAnExistingSaveWhereSuspendIsNotAllowed(t *testing.T) {
	ctx := testContext(t)
	for name, set := range map[string]func(c *Crawl){
		"battle": func(c *Crawl) { testBattle(c, c.level.Monsters[0]) },
		"puzzle": func(c *Crawl) { c.mode = modePuzzle },
		"fall":   func(c *Crawl) { c.die() },
	} {
		useTempDir(t)
		writeTestSave(t, ctx)
		before, _ := save.Read(saveName)
		c := testCrawl(t, ctx)
		c.run.hero.Gold = 999
		set(c)
		c.OnClose(ctx)
		after, err := save.Read(saveName)
		if err != nil || string(after) != string(before) {
			t.Errorf("%s: the saved game was changed or removed", name)
		}
	}
}

// Hardcore and Daily suspends written on close are picked up once.
func TestCloseSuspendIsPickedUpOnce(t *testing.T) {
	for _, mode := range []compete.Mode{compete.Hardcore, compete.Daily} {
		useTempDir(t)
		ctx := testContext(t)
		c := testCrawl(t, ctx)
		c.run.setMode(ctx, mode)
		c.OnClose(ctx)
		if !hasSaveFile() {
			t.Fatalf("%v: not suspended", mode)
		}
		got, err := loadCrawl(ctx)
		if err != nil {
			t.Fatalf("%v: %v", mode, err)
		}
		if got.run.mode != mode {
			t.Errorf("%v: resumed as %v", mode, got.run.mode)
		}
		if hasSaveFile() {
			t.Errorf("%v: still on disk after resuming", mode)
		}
		if _, err := loadCrawl(ctx); err == nil {
			t.Errorf("%v: picked up twice", mode)
		}
	}
}

// A normal Adventure close is a plain save that Continue loads, and
// resuming it does not delete it.
func TestCloseAdventureSaveSurvivesContinue(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	c.OnClose(ctx)
	c.OnClose(ctx)
	for i := 0; i < 2; i++ {
		if _, err := loadCrawl(ctx); err != nil {
			t.Fatalf("load %d: %v", i, err)
		}
		if !hasSaveFile() {
			t.Fatalf("load %d: an Adventure save was deleted", i)
		}
	}
	// Continued play: afterClose must not touch an Adventure.
	c.afterClose(time.Now().Add(time.Hour))
	if !hasSaveFile() {
		t.Fatal("afterClose removed an Adventure save")
	}
}

func TestGameOverCloseDoesNotRecordTwiceOrAfterEntry(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	c := hardcoreCrawl(t, ctx, 3)
	c.die()
	g := newGameOver(ctx, c.run, false)
	if g.place == 0 {
		t.Fatal("expected a place")
	}
	key := compete.TableKey(compete.Hardcore, "fr")
	g.name = []rune("Zed")
	g.record(ctx) // the player pressed Enter
	g.name = []rune("Other")
	g.OnClose(ctx)
	g.OnClose(ctx)
	table := ctx.Profile.Fame.Table(key)
	if len(table) != 1 || table[0].Name != "Zed" {
		t.Fatalf("hall of fame %+v", table)
	}
}
