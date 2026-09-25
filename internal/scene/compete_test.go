package scene

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/profile"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/pkg/compete"
	"github.com/halpworld/halpwords/pkg/puzzle"
	"github.com/halpworld/halpwords/pkg/words"
)

// hardcoreCrawl is a Hardcore run on floor depth.
func hardcoreCrawl(t *testing.T, ctx *game.Context, depth int) *Crawl {
	t.Helper()
	fr, _ := words.Lookup("fr")
	r := newRun(ctx, fr, rpg.Knight, runSetup{mode: compete.Hardcore, seed: 4242, seeded: true})
	r.sound = &game.Sound{Muted: true}
	r.depth = depth
	return crawlOn(r, r.floor(depth))
}

func TestHardcoreFloorsAndRules(t *testing.T) {
	ctx := testContext(t)
	fr, _ := words.Lookup("fr")
	// The player's own settings don't apply to Hardcore runs.
	ls := profile.Preset(fr)
	ls.Rules.Accents, ls.Timer, ls.Highlight = words.Ignore, profile.Relaxed, true
	ctx.Profile.Settings.Set(fr, ls)
	c := hardcoreCrawl(t, ctx, 2)
	if c.run.settings != profile.Preset(fr) {
		t.Fatalf("Hardcore settings %+v", c.run.settings)
	}
	if c.run.seed != 4242 || c.run.seedCode() != compete.SeedCode(4242) {
		t.Fatal("the seed challenge's seed was not used")
	}
	for _, ft := range c.level.Features {
		if ft.Kind == dungeon.Shrine {
			t.Fatal("a shrine on a Hardcore floor")
		}
	}
	// Adventure runs use the player's settings.
	a := newRun(ctx, fr, rpg.Knight, runSetup{})
	if a.settings != ls || a.mode != compete.Adventure {
		t.Fatalf("Adventure settings %+v", a.settings)
	}
	if a.seed>>compete.SeedBits != 0 {
		t.Fatal("a new run's seed has no seed code")
	}
}

func TestHardcoreShopHasNoTimeItems(t *testing.T) {
	ctx := testContext(t)
	c := hardcoreCrawl(t, ctx, 2)
	c.feature = &dungeon.Feature{Kind: dungeon.Merchant}
	c.menu = &menu{shop: true}
	for _, row := range c.shopRows() {
		if row.label == rpg.Hourglass.String() || row.label == rpg.Clarity.String() {
			t.Fatalf("a Hardcore merchant sells %s", row.label)
		}
	}
}

func TestScoreTally(t *testing.T) {
	ctx := testContext(t)
	c := hardcoreCrawl(t, ctx, 1)
	_, id := c.run.deck.Next()
	for i := 0; i < 3; i++ {
		c.scoreAnswer(id, words.Result{Tier: words.Perfect}, "", false, 1)
	}
	c.scoreAnswer(id, words.Result{Tier: words.Miss, Expected: "le chien"}, "le chein", false, 1)
	c.scoreAnswer(id, words.Result{Tier: words.Correct}, "", false, 1)
	tl := c.run.tally
	if tl.Perfect != 3 || tl.BestCombo != 3 || tl.Misses != 1 {
		t.Fatalf("tally %+v", tl)
	}
	m := dungeon.NewMonster(&dungeon.Kinds[0], 1, c.pos, 1)
	m.HP = 3
	b := testBattle(c, m)
	b.field.Type('x') // a miss: no damage
	c.strike(ctx)
	if c.run.tally.Damage != 0 {
		t.Fatal("a miss counted as damage")
	}
	if want := 1000 + 3*50 + 3*100 - 2*25; c.run.score() != want {
		t.Fatalf("score %d, want %d", c.run.score(), want)
	}
}

func TestDamageIsCappedAtTheMonstersHP(t *testing.T) {
	ctx := testContext(t)
	c := hardcoreCrawl(t, ctx, 1)
	m := dungeon.NewMonster(&dungeon.Kinds[0], 1, c.pos, 1)
	m.HP = 2
	b := testBattle(c, m)
	for _, r := range b.word.Answers[0] {
		b.field.Type(r)
	}
	c.strike(ctx)
	if c.run.tally.Damage != 2 {
		t.Fatalf("damage %d counted against a monster with 2 HP", c.run.tally.Damage)
	}
}

func TestAnswersGoInTheGrimoire(t *testing.T) {
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	e, id := c.run.deck.Next()
	c.scoreAnswer(id, words.Result{Tier: words.Graze, Expected: e.Answers[0]}, "x"+e.Answers[0], false, 2.5)
	card := ctx.Profile.MemoryFor("fr").Card(e)
	if card == nil || card.Seen != 1 || card.Timed != 1 || card.Box != 1 {
		t.Fatalf("card %+v", card)
	}
	if card.Mistakes[words.ExtraMistake] != 1 {
		t.Fatalf("mistakes %v", card.Mistakes)
	}
	// Another adventure knows the word.
	r := startRun(ctx, c.run.lang, rpg.Scribe, 7)
	if r.deck.Memory().Box(e) != 1 {
		t.Fatal("a new adventure forgot the word")
	}
}

func TestSettingsChangeGrading(t *testing.T) {
	ctx := testContext(t)
	fr, _ := words.Lookup("fr")
	ls := profile.Preset(fr)
	ls.Rules.Accents, ls.Timer = words.Strict, profile.Fast
	ctx.Profile.Settings.Set(fr, ls)
	r := newRun(ctx, fr, rpg.Knight, runSetup{seed: 1, seeded: true})
	c := crawlOn(r, r.floor(1))
	m := dungeon.NewMonster(&dungeon.Kinds[0], 1, c.pos, 1)
	b := testBattle(c, m)
	b.word = words.Entry{Prompt: "school", Answers: []string{"l'école"}}
	if got := c.grade("l'ecole").Tier; got != words.Miss {
		t.Fatalf("strict accents: %v", got)
	}
	if got := c.timeScale(); got != c.run.hero.TimeBonus()*0.75 {
		t.Fatalf("fast timers: time scale %v", got)
	}
	// Puzzles are graded by the same rules.
	c.run.depth = 2
	lp := &lockPuzzle{lock: puzzle.Door}
	c.puzzle = lp
	for i := 0; i < 2000; i++ {
		c.dealPuzzle()
		if want := puzzleAnswer(lp.p); lp.p.Kind() == puzzle.Spell && strings.Contains(want, "é") {
			res := lp.p.Check(puzzle.Attempt{Text: strings.ReplaceAll(want, "é", "e")})
			if res.Passed() {
				t.Fatalf("a strict puzzle passed %q without its accent: %v", want, res.Tier)
			}
			return
		}
	}
	t.Fatal("no spelling puzzle with an accent")
}

func TestHardcoreDeathEndsTheRun(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	c := hardcoreCrawl(t, ctx, 2)
	if !c.writeSave(ctx, true) {
		t.Fatal("could not suspend")
	}
	loaded, err := loadCrawl(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := save.Read(saveName); err == nil {
		t.Fatal("a Hardcore save was kept after loading it")
	}
	if !loaded.run.hardcore() || loaded.run.seed != 4242 {
		t.Fatal("the Hardcore run did not come back")
	}
	if !reflect.DeepEqual(loaded.run.tally, c.run.tally) {
		t.Fatal("the tally did not come back")
	}
	loaded.die()
	if loaded.mode != modeDead {
		t.Fatal("not dead")
	}
	g := newGameOver(ctx, loaded.run, false)
	if g.score != loaded.run.score() || g.place != 1 {
		t.Fatalf("game over: score %d place %d", g.score, g.place)
	}
	g.name = []rune("Ada")
	g.record(ctx)
	table := ctx.Profile.Fame.Table(compete.TableKey(compete.Hardcore, "fr"))
	if len(table) != 1 || table[0].Name != "Ada" || table[0].Code != g.code {
		t.Fatalf("hall of fame %+v", table)
	}
	s, err := compete.ParseShare(g.code)
	if err != nil || s.Seed != 4242 || s.Floor != 2 || s.Score != g.score {
		t.Fatalf("share code %q: %+v %v", g.code, s, err)
	}
}

func TestHardcoreSaveWithoutSuspendIsOver(t *testing.T) {
	ctx := testContext(t)
	c := hardcoreCrawl(t, ctx, 1)
	data, err := encodeSave(c.run, c.level, c.pos, c.facing, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeSave(ctx, data); err == nil {
		t.Fatal("loaded a Hardcore run with no suspended game")
	}
}

func TestDailyDungeon(t *testing.T) {
	ctx := testContext(t)
	fr, _ := words.Lookup("fr")
	a, b := dailySetup(ctx, fr), dailySetup(ctx, fr)
	if a != b || a.mode != compete.Daily || !a.seeded || a.day != time.Now().Format(time.DateOnly) {
		t.Fatalf("daily setups %+v %+v", a, b)
	}
	la, _ := words.Lookup("la")
	if dailySetup(ctx, la).seed == a.seed {
		t.Fatal("Latin and French share a daily dungeon")
	}
	r := newRun(ctx, fr, rpg.Rogue, a)
	r.sound = &game.Sound{Muted: true}
	g := newGameOver(ctx, r, true)
	s, err := compete.ParseShare(g.code)
	if err != nil || !s.Daily || s.Day != time.Now().Day() {
		t.Fatalf("daily share code %q: %+v %v", g.code, s, err)
	}
}

func TestOldSavesAreAdventures(t *testing.T) {
	ctx := testContext(t)
	r, l := testRun(t, ctx)
	data, err := encodeSave(r, l, l.Start, dungeon.West, true)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeSave(ctx, data)
	if err != nil || got.run.mode != compete.Adventure || got.run.hardcore() {
		t.Fatalf("mode %v, %v", got.run.mode, err)
	}
}
