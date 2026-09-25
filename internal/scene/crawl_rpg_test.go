package scene

import (
	"strings"
	"testing"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/internal/typing"
	"github.com/halpworld/halpwords/internal/words"
)

// testBattle starts a fight with m on the hero's turn.
func testBattle(c *Crawl, m *dungeon.Monster) *battle {
	c.mode = modeBattle
	c.battle = &battle{m: m, field: typing.NewField(c.run.lang)}
	c.battle.word, c.battle.wordID = c.run.deck.Next()
	c.battle.phase = phaseAttack
	return c.battle
}

func TestBossEnrages(t *testing.T) {
	c := testCrawl(t, testContext(t))
	m := dungeon.NewMonster(dungeon.BossFor(3), 3, c.pos, 1)
	testBattle(c, m)
	if lines := c.enrage(m); len(lines) != 0 || m.Phase != 0 {
		t.Fatal("a boss at full health enraged")
	}
	m.HP = m.MaxHP / 2
	c.enrage(m)
	if m.Phase != 1 || !m.Has(dungeon.Swift) {
		t.Fatalf("phase %d traits %v at half health", m.Phase, m.Traits)
	}
	m.HP = 1
	c.enrage(m)
	if m.Phase != 2 || !m.Has(dungeon.Mirrored) {
		t.Fatalf("phase %d traits %v near death", m.Phase, m.Traits)
	}
	// Enraged bosses ask for longer words when the lists have them.
	c.deal(testContext(t), phaseAttack)
	if n := runes(c.battle.word.Answers[0]); n < 6 {
		t.Fatalf("an angry boss asked for %q", c.battle.word.Answers[0])
	}
}

func TestBossHoldsTheStairs(t *testing.T) {
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	c.run.depth = 3
	c.level = dungeon.Generate(c.run.floorSeed(3), 3)
	if c.level.Boss() == nil {
		t.Fatal("no boss on floor 3")
	}
	c.pos = c.level.Exit
	c.descend(ctx)
	if c.run.depth != 3 {
		t.Fatal("went down the stairs past a living boss")
	}
}

func TestDefenceSoftensBlows(t *testing.T) {
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	m := dungeon.NewMonster(&dungeon.Kinds[0], 1, c.pos, 1)
	m.ATK = 10
	testBattle(c, m)
	hp := c.run.hero.HP
	c.dodge(ctx, words.Result{Tier: words.Miss, Expected: "x"}, "", 3)
	lost := hp - c.run.hero.HP
	def := c.run.hero.DEF()
	if lost < 9-def || lost > 11-def {
		t.Fatalf("lost %d HP to a blow of 9-11 with DEF %d", lost, def)
	}
}

func TestHints(t *testing.T) {
	c := testCrawl(t, testContext(t))
	h := &c.run.hero
	if got := hintText("le chien", 3); got != "l e   c _ _ _ _" {
		t.Fatalf("hint %q", got)
	}
	if got := hintText("l'eau", 9); got != "l ' e a u" {
		t.Fatalf("hint %q", got)
	}
	h.MP, h.Items[rpg.HintScroll] = 3, 1
	shown := 0
	c.hint(&shown, "chien") // 2 MP
	c.hint(&shown, "chien") // a scroll: only 1 MP left
	c.hint(&shown, "chien") // nothing left
	if shown != 2 || h.MP != 1 || h.Items[rpg.HintScroll] != 0 {
		t.Fatalf("shown %d, MP %d, scrolls %d", shown, h.MP, h.Items[rpg.HintScroll])
	}
	// A hinted word is practised again even when spelled right.
	_, id := c.run.deck.Next()
	c.scoreAnswer(id, words.Result{Tier: words.Perfect}, "", true, 0)
	if c.run.deck.Review() != 1 || c.run.perfect[id] {
		t.Fatal("a hinted word counted as known")
	}
	c.scoreAnswer(id, words.Result{Tier: words.Perfect}, "", false, 0)
	if c.run.deck.Review() != 0 || !c.run.perfect[id] || h.XP != rpg.PerfectXP {
		t.Fatalf("a perfect word: review %d, XP %d", c.run.deck.Review(), h.XP)
	}
	c.scoreAnswer(id, words.Result{Tier: words.Perfect}, "", false, 0)
	if h.XP != rpg.PerfectXP {
		t.Fatal("a word's first perfect spelling paid twice")
	}
}

func TestGoodPrefix(t *testing.T) {
	fr, _ := words.Lookup("fr")
	answers := []string{"le chien"}
	for typed, want := range map[string]int{"": 0, "Chi": 3, "chix": 3, "le c": 4, "x": 0, "chiens": 5} {
		if got := goodPrefix(typed, answers, fr); got != want {
			t.Errorf("%q: %d letters good, want %d", typed, got, want)
		}
	}
}

// Using an item from the menu in a battle takes the hero's turn.
func TestItemsInBattle(t *testing.T) {
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	testBattle(c, c.level.Monsters[0])
	h := &c.run.hero
	h.HP, h.Items[rpg.Clarity] = 5, 1
	c.pause(ctx)
	c.choose(ctx, pauseItems)
	if c.mode != modeItems || c.resume != modeBattle {
		t.Fatalf("mode %d resume %d", c.mode, c.resume)
	}
	c.useItem(rpg.Clarity)
	c.closeMenu(ctx)
	if !c.battle.clarity || h.Items[rpg.Clarity] != 0 {
		t.Fatal("the rune did not work")
	}
	if c.mode != modeBattle || c.battle.phase != phaseDefend {
		t.Fatalf("mode %d phase %d: using an item did not take the turn", c.mode, c.battle.phase)
	}
	// Outside battles, the Hourglass waits for the next one.
	c.battle, c.mode = nil, modeExplore
	h.Items[rpg.Hourglass] = 1
	c.openItems(ctx)
	c.useItem(rpg.Hourglass)
	c.closeMenu(ctx)
	if !h.Hourglass || c.mode != modeExplore {
		t.Fatal("the hourglass is not ready for the next battle")
	}
	c.startBattle(ctx, c.level.Monsters[0], false)
	if !c.battle.slow || h.Hourglass {
		t.Fatal("the hourglass did not turn in the battle")
	}
}

func TestShop(t *testing.T) {
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	h := &c.run.hero
	h.Gold = 1000
	sword := rpg.Gear{Slot: rpg.Weapon, Tier: 2, Base: 2}
	c.feature = &dungeon.Feature{Kind: dungeon.Merchant, Stock: []rpg.Gear{sword}}
	c.openShop(ctx)
	buy := func(label string) {
		t.Helper()
		for _, row := range c.menu.rows {
			if row.label == label && row.act != nil {
				row.act(ctx)
				c.buildMenu()
				return
			}
		}
		t.Fatalf("can't choose %q", label)
	}
	buy("Ether")
	if h.Items[rpg.Ether] != 1 || h.Gold != 1000-rpg.Ether.Info().Price {
		t.Fatalf("%d ethers, %d gold", h.Items[rpg.Ether], h.Gold)
	}
	gold := h.Gold
	buy(sword.Name())
	if len(c.feature.Stock) != 0 || len(h.Bag) != 1 || h.Gold != gold-sword.Price() {
		t.Fatalf("stock %v, bag %v, gold %d", c.feature.Stock, h.Bag, h.Gold)
	}
	gold = h.Gold
	buy(sword.Name()) // now the bag's, to sell
	if len(h.Bag) != 0 || h.Gold != gold+sword.SellPrice() {
		t.Fatalf("selling: bag %v gold %d", h.Bag, h.Gold)
	}
	h.Gold = 0
	c.buildMenu()
	for _, row := range c.menu.rows {
		if row.act != nil {
			t.Fatalf("can buy %q with no gold", row.label)
		}
	}
}

func TestCampfire(t *testing.T) {
	c := testCrawl(t, testContext(t))
	h := &c.run.hero
	h.HP, h.MP = 1, 0
	_, id := c.run.deck.Next()
	c.run.deck.Mark(id, false)
	ft := &dungeon.Feature{Kind: dungeon.Campfire}
	c.rest(ft)
	if h.HP != h.MaxHP() || h.MP != h.MaxMP() || !ft.Used || c.mode != modeCampfire {
		t.Fatalf("rest: HP %d MP %d used %v", h.HP, h.MP, ft.Used)
	}
	e := c.run.deck.Entries()[id]
	if len(c.weakest) != 1 || !strings.Contains(c.weakest[0].text, e.Answers[0]) {
		t.Fatalf("the fire shows %v", c.weakest)
	}
	h.HP = 1
	c.mode = modeExplore
	c.rest(ft)
	if h.HP != 1 {
		t.Fatal("a burnt-out campfire healed")
	}
}

func TestLoot(t *testing.T) {
	c := testCrawl(t, testContext(t))
	h := &c.run.hero
	ring := rpg.Gear{Slot: rpg.Trinket, Affix: rpg.Owl}
	ch := &dungeon.Chest{Potions: 2, Items: []rpg.Item{rpg.Ether}, Gear: &ring}
	lines := c.takeLoot(ch, "Inside: ")
	if h.Items[rpg.Potion] != 3 || h.Items[rpg.Ether] != 1 || h.Gear[rpg.Trinket] == nil || ch.Gear != nil {
		t.Fatalf("loot not taken: %+v", h)
	}
	if len(lines) != 2 || lines[0].text != "Inside: 2 Potions and an Ether!" {
		t.Fatalf("lines %v", lines)
	}
	// Gear that does not fit stays in the chest for later.
	for len(h.Bag) < rpg.BagSize {
		h.Bag = append(h.Bag, ring)
	}
	ch.Gear = &ring
	c.takeLoot(ch, "")
	if ch.Gear == nil {
		t.Fatal("gear vanished when the bag was full")
	}
	h.Bag = nil
	c.emptyChest(ch)
	if ch.Gear != nil || len(h.Bag) != 1 {
		t.Fatal("the gear left behind could not be taken later")
	}
}
