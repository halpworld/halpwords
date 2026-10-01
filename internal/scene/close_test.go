package scene

import (
	"testing"
	"time"

	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/pkg/compete"
)

func hasSaveFile() bool {
	_, err := save.Read(saveName)
	return err == nil
}

// Closing the game while exploring keeps the Adventure (#49): it comes
// back where the hero stood.
func TestCloseSavesAnAdventure(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	c.run.hero.Gold = 41
	c.OnClose(ctx)
	data, err := save.Read(saveName)
	if err != nil {
		t.Fatal("nothing was saved on close")
	}
	got, err := decodeSave(ctx, data)
	if err != nil || !got.suspended || got.at != c.pos || got.run.hero.Gold != 41 {
		t.Fatalf("loaded %+v, %v", got, err)
	}
}

// A normal close of a Hardcore run is a suspend, so Continue finds it.
func TestCloseSuspendsAHardcoreRun(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	c := hardcoreCrawl(t, ctx, 2)
	c.OnClose(ctx)
	if !hasSaveFile() {
		t.Fatal("the Hardcore run was not suspended on close")
	}
	if _, err := loadCrawl(ctx); err != nil {
		t.Fatal(err)
	}
	if hasSaveFile() {
		t.Fatal("a resumed Hardcore run stayed on disk")
	}
}

// On the web the page is hidden, not closed, and the game goes on when
// it is shown again: a Hardcore run is then back to being in memory only,
// so it can still be picked up once and no more.
func TestHardcoreCloseWriteGoesWhenThePlayerReturns(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	c := hardcoreCrawl(t, ctx, 2)
	c.OnClose(ctx)
	c.afterClose(ctx, time.Now()) // a frame that still runs as the page is hidden
	if !hasSaveFile() {
		t.Fatal("the suspend was taken back at once, so a closed tab loses the run")
	}
	c.afterClose(ctx, time.Now().Add(2*closeGrace))
	if hasSaveFile() || c.run.onDisk {
		t.Fatal("the Hardcore suspend was kept after the player came back")
	}
	// An Adventure keeps what was written.
	useTempDir(t)
	a := testCrawl(t, ctx)
	a.OnClose(ctx)
	a.afterClose(ctx, time.Now().Add(2*closeGrace))
	if !hasSaveFile() {
		t.Fatal("the Adventure save went")
	}
}

// Closing mid-battle, mid-puzzle, in a race or when dead writes nothing:
// the pause menu doesn't offer Suspend there either.
func TestCloseWritesNothingWhereSuspendIsNotAllowed(t *testing.T) {
	ctx := testContext(t)
	for name, set := range map[string]func(c *Crawl){
		"battle": func(c *Crawl) {
			c.mode = modeBattle
			c.battle = testBattle(c, c.level.Monsters[0])
		},
		"paused in a battle": func(c *Crawl) {
			c.mode = modeBattle
			c.battle = testBattle(c, c.level.Monsters[0])
			c.pause(ctx)
		},
		"puzzle": func(c *Crawl) { c.mode = modePuzzle },
		"dead":   func(c *Crawl) { c.mode = modeDead },
		"a race": func(c *Crawl) { c.run.race = &raceRun{goal: 5} },
	} {
		for _, hard := range []bool{false, true} {
			useTempDir(t)
			c := testCrawl(t, ctx)
			if hard {
				c.run.setMode(ctx, compete.Hardcore)
			}
			set(c)
			c.OnClose(ctx)
			if hasSaveFile() {
				t.Errorf("%s (hardcore %v): wrote a save", name, hard)
			}
		}
	}
}

// The pause menu over exploring is a place Suspend works.
func TestCloseFromThePauseMenu(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	c.pause(ctx)
	c.mode = modeQuit
	c.OnClose(ctx)
	if !hasSaveFile() {
		t.Fatal("not saved from the pause menu")
	}
}

// A score that earned a place in the Hall of Fame is kept when the game
// closes at the name prompt (#50), under the name typed so far, or the
// last name used.
func TestCloseAtTheNamePromptRecordsTheRun(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	c := hardcoreCrawl(t, ctx, 3)
	c.die()
	g := newGameOver(ctx, c.run, false)
	if g.place != 1 {
		t.Fatalf("place %d, want 1", g.place)
	}
	key := compete.TableKey(compete.Hardcore, "fr")
	g.name = []rune("Ad")
	g.OnClose(ctx)
	g.OnClose(ctx) // the web build can say it twice: hidden, then gone
	table := ctx.Profile.Fame.Table(key)
	if len(table) != 1 || table[0].Name != "Ad" || table[0].Score != g.score {
		t.Fatalf("hall of fame %+v", table)
	}

	// With nothing typed, and no name used before, the entry is "Hero".
	ctx2 := testContext(t)
	h := newGameOver(ctx2, c.run, false)
	h.name = nil
	h.OnClose(ctx2)
	if t2 := ctx2.Profile.Fame.Table(key); len(t2) != 1 || t2[0].Name != "Hero" {
		t.Fatalf("hall of fame %+v", t2)
	}
}

func TestCloseAtGameOverWithoutAPlaceRecordsNothing(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	c := hardcoreCrawl(t, ctx, 1)
	c.run.tally = compete.Tally{}
	g := newGameOver(ctx, c.run, true)
	g.place = 0
	g.OnClose(ctx)
	if len(ctx.Profile.Fame.Table(compete.TableKey(compete.Hardcore, "fr"))) != 0 {
		t.Fatal("recorded a run that did not place")
	}
}

// A suspend written when the page was hidden is stale once the player
// comes back: dying and then closing the tab must not bring back the
// state from before the fall (#49).
func TestAdventureSuspendIsNotStaleAfterReturning(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	c.OnClose(ctx) // page hidden
	c.afterClose(ctx, time.Now().Add(2*closeGrace))
	c.die()
	c.OnClose(ctx) // tab closed at the death screen: nothing to write
	data, err := save.Read(saveName)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeSave(ctx, data)
	if err != nil || got.suspended {
		t.Fatalf("Continue would load the suspend from before the fall: %v", err)
	}
}

// At the name prompt a hidden page keeps a provisional entry, leaves the
// prompt open, and entering the name renames that entry (#50).
func TestHiddenAtTheNamePromptKeepsThePromptOpen(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	ctx.Profile.Name = "Old"
	c := hardcoreCrawl(t, ctx, 3)
	c.die()
	g := newGameOver(ctx, c.run, false)
	key := compete.TableKey(compete.Hardcore, "fr")
	g.name = []rune("Ad")
	g.OnHide(ctx)
	g.OnHide(ctx)
	table := ctx.Profile.Fame.Table(key)
	if len(table) != 1 || table[0].Name != "Ad" {
		t.Fatalf("hall of fame %+v", table)
	}
	if g.entered || ctx.Profile.Name != "Old" {
		t.Fatal("the prompt closed, or the profile's name changed, on hidden")
	}
	g.name = []rune("Ada")
	g.record(ctx) // Enter
	table = ctx.Profile.Fame.Table(key)
	if len(table) != 1 || table[0].Name != "Ada" || !g.entered || ctx.Profile.Name != "Ada" {
		t.Fatalf("hall of fame %+v", table)
	}
	// A close with the prompt still open keeps the typed name.
	c.run.depth = 4 // another run, with its own share code
	h := newGameOver(ctx, c.run, false)
	h.name = []rune("Bo")
	h.OnHide(ctx)
	h.name = []rune("Bob")
	h.OnClose(ctx)
	if got := ctx.Profile.Fame.Table(key); len(got) != 2 || got[0].Name+got[1].Name != "AdaBob" && got[0].Name+got[1].Name != "BobAda" {
		t.Fatalf("hall of fame %+v", got)
	}
}
