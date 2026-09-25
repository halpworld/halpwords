package scene

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/halpworld/halpwords/assets"
	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/profile"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/pkg/words"
)

func testContext(t *testing.T) *game.Context {
	t.Helper()
	lists, err := words.LoadFS(assets.Words, "words")
	if err != nil {
		t.Fatal(err)
	}
	return &game.Context{Lists: lists, Profile: profile.New(), Sound: &game.Sound{Muted: true}}
}

// useTempDir points the save folder at a fresh temporary folder.
func useTempDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("AppData", dir)
}

// testRun is a run a little way into an adventure.
func testRun(t *testing.T, ctx *game.Context) (*run, *dungeon.Level) {
	t.Helper()
	lang, _ := words.Lookup("fr")
	r := startRun(ctx, lang, rpg.Rogue, 99)
	r.depth, r.regen = 3, 1
	r.hero.Gold, r.hero.Items[rpg.Potion], r.hero.Streak = 17, 2, 4
	r.hero.Take(rpg.Gear{Slot: rpg.Trinket, Tier: 1, Affix: rpg.Fortune})
	r.hero.Bag = append(r.hero.Bag, rpg.Gear{Slot: rpg.Armor, Tier: 2})
	r.hero.Hourglass = true
	r.shrine.Hero.Gold = 5
	r.seenTraits = dungeon.Swift | dungeon.Armored
	r.perfect[3] = true
	r.say("A test line.", pal.Lime)
	for n := 0; n < 5; n++ {
		_, id := r.deck.Next()
		r.deck.Mark(id, n%2 == 0)
	}
	l := dungeon.Generate(r.floorSeed(r.depth), r.depth)
	l.Seen[l.Index(l.Start)] = true
	l.Monsters[0].HP = 1
	return r, l
}

// A suspended run must load back exactly as it was, down to the next
// random number.
func TestSaveRoundTrip(t *testing.T) {
	ctx := testContext(t)
	r, l := testRun(t, ctx)
	data, err := encodeSave(r, l, l.Start, dungeon.West, true)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeSave(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	g := got.run
	switch {
	case !got.suspended:
		t.Fatal("a suspended game did not load as one")
	case g.depth != r.depth || g.regen != r.regen || g.seed != r.seed || g.lang != r.lang:
		t.Fatalf("got depth %d regen %d seed %d lang %s", g.depth, g.regen, g.seed, g.lang.Code)
	case !reflect.DeepEqual(g.hero, r.hero):
		t.Fatalf("got hero %+v, want %+v", g.hero, r.hero)
	case !reflect.DeepEqual(g.shrine, r.shrine):
		t.Fatalf("got shrine %+v, want %+v", g.shrine, r.shrine)
	case g.seenTraits != r.seenTraits:
		t.Fatalf("got traits %v", g.seenTraits)
	case !reflect.DeepEqual(g.perfect, r.perfect):
		t.Fatalf("got perfect words %v", g.perfect)
	case !reflect.DeepEqual(g.log, r.log):
		t.Fatalf("got log %v", g.log)
	case !reflect.DeepEqual(g.deck.State(), r.deck.State()):
		t.Fatalf("got deck %+v, want %+v", g.deck.State(), r.deck.State())
	case !reflect.DeepEqual(got.level, l):
		t.Fatal("floor differs")
	case got.at != l.Start || got.facing != dungeon.West:
		t.Fatalf("got hero at %v facing %v", got.at, got.facing)
	}
	for n := 0; n < 10; n++ {
		if a, b := r.rng.Uint64(), g.rng.Uint64(); a != b {
			t.Fatalf("random numbers differ after loading: %d, %d", a, b)
		}
	}
}

// Without a suspended game, loading goes back to the shrine: the hero and
// the floor as they were when the hero prayed.
func TestLoadAtShrine(t *testing.T) {
	ctx := testContext(t)
	r, l := testRun(t, ctx)
	l.Monsters[1].HP = 0
	r.shrine = r.here(l, l.Start, dungeon.South)
	shrineHero := r.hero.Clone()
	r.hero.Gold = 999 // after praying: not saved
	data, err := encodeSave(r, l, l.Start, l.StartDir, false)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeSave(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	switch {
	case got.suspended:
		t.Fatal("loaded a suspended game from a shrine save")
	case !reflect.DeepEqual(got.run.hero, shrineHero):
		t.Fatalf("got hero %+v, want the one at the shrine", got.run.hero)
	case !reflect.DeepEqual(got.level, l):
		t.Fatal("the floor is not as it was at the shrine")
	case got.facing != dungeon.South:
		t.Fatalf("facing %v", got.facing)
	}
}

func TestLoadBadSave(t *testing.T) {
	ctx := testContext(t)
	lang, _ := words.Lookup("la")
	r := startRun(ctx, lang, rpg.Knight, 1)
	l := dungeon.Generate(r.floorSeed(1), 1)
	good, err := encodeSave(r, l, l.Start, l.StartDir, true)
	if err != nil {
		t.Fatal(err)
	}
	change := func(f func(s map[string]any)) []byte {
		var s map[string]any
		if err := json.Unmarshal(good, &s); err != nil {
			t.Fatal(err)
		}
		f(s)
		data, _ := json.Marshal(s)
		return data
	}
	suspend := func(s map[string]any) map[string]any { return s["Suspend"].(map[string]any) }
	for name, data := range map[string][]byte{
		"not json":      []byte("{nope"),
		"old version":   change(func(s map[string]any) { s["Version"] = 1 }),
		"no language":   change(func(s map[string]any) { s["Language"] = "xx" }),
		"no shrine":     change(func(s map[string]any) { s["Shrine"].(map[string]any)["Depth"] = 0 }),
		"no floor":      change(func(s map[string]any) { suspend(s)["Depth"] = 0 }),
		"wrong floor":   change(func(s map[string]any) { suspend(s)["Depth"] = 9 }),
		"in a wall":     change(func(s map[string]any) { suspend(s)["At"] = map[string]int{"X": 0, "Y": 0} }),
		"bad generator": change(func(s map[string]any) { s["RNG"] = "" }),
	} {
		if _, err := decodeSave(ctx, data); err == nil {
			t.Errorf("%s: loaded without an error", name)
		}
	}
	if _, err := decodeSave(ctx, good); err != nil {
		t.Fatalf("good save: %v", err)
	}
}

// A suspended game can be continued once; after that, Continue goes back
// to the shrine.
func TestSuspendIsUsedOnce(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	c.run.hero.Gold = 50
	c.pos = c.level.Start
	if !c.writeSave(ctx, true) {
		t.Fatal("could not save")
	}
	c2, err := loadCrawl(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if c2.run.hero.Gold != 50 {
		t.Fatalf("suspended game has %d gold", c2.run.hero.Gold)
	}
	if s, _ := saveSummary(); s != "French · Knight · Floor 1" {
		t.Fatalf("summary %q", s)
	}
	c3, err := loadCrawl(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if c3.run.hero.Gold != 0 {
		t.Fatalf("the suspended game loaded twice: %d gold", c3.run.hero.Gold)
	}
}

// Falling wakes the hero at their last shrine, with a new floor and a
// fifth of their gold gone, and the save on disk follows.
func TestFallingWakesAtShrine(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	c.run.depth = 3
	c.level = dungeon.Generate(c.run.floorSeed(3), 3)
	c.pos = c.level.Start
	c.run.hero.Gold = 100
	c.pray(ctx)
	if !c.run.onDisk {
		t.Fatal("praying did not save")
	}
	c.run.hero.Gold = 150
	c.run.hero.Items[rpg.Potion] = 9
	c.run.depth = 4 // went further down, then fell
	c.run.deck.Mark(0, false)
	c.die()
	next := c.woken(ctx)
	r := next.run
	switch {
	case r.depth != 3 || r.regen != 1:
		t.Fatalf("woke on floor %d regen %d", r.depth, r.regen)
	case r.hero.Gold != 80 || r.hero.Items[rpg.Potion] != 1:
		t.Fatalf("woke with %d gold and %d potions", r.hero.Gold, r.hero.Items[rpg.Potion])
	case r.hero.HP != r.hero.MaxHP():
		t.Fatal("woke hurt")
	case r.deck.Review() == 0:
		t.Fatal("falling forgot the missed words")
	case reflect.DeepEqual(next.level.State().Tiles, c.level.State().Tiles):
		t.Fatal("the floor was not made anew")
	}
	loaded, err := loadCrawl(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.run.hero.Gold != 80 || loaded.run.regen != 1 || loaded.run.depth != 3 {
		t.Fatal("the save on disk does not know about the fall")
	}
}

// A new adventure that has never been saved must not overwrite the save
// of another when the hero falls.
func TestFallingDoesNotSaveNewRun(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	c.die()
	c.woken(ctx)
	if _, err := save.Read(saveName); err == nil {
		t.Fatal("falling saved an adventure that was never saved")
	}
}
