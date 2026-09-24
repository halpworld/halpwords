package rpg

import (
	"encoding/json"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
)

func TestNewHero(t *testing.T) {
	for _, c := range Classes {
		h := NewHero(c)
		if h.HP != h.MaxHP() || h.MP != h.MaxMP() || h.HP <= 0 {
			t.Errorf("%s: starts with HP %d/%d MP %d/%d", c, h.HP, h.MaxHP(), h.MP, h.MaxMP())
		}
		if h.Gear[Weapon] == nil || h.ATK() <= h.Base.ATK {
			t.Errorf("%s: no starting weapon", c)
		}
		if h.Items[Potion] != 1 {
			t.Errorf("%s: %d potions", c, h.Items[Potion])
		}
	}
	k, s, r := NewHero(Knight), NewHero(Scribe), NewHero(Rogue)
	if k.MaxHP() <= s.MaxHP() || k.DEF() <= s.DEF() {
		t.Error("the knight is not the toughest")
	}
	if s.MaxMP() <= k.MaxMP() || s.HintCost() >= k.HintCost() {
		t.Error("the scribe has no magic edge")
	}
	if r.DodgeBonus() <= k.DodgeBonus() || r.GoldFind(100, true) <= k.GoldFind(100, true) {
		t.Error("the rogue has no dodge or loot edge")
	}
	if s.SpeedDamage(2) <= k.SpeedDamage(2) || s.SpeedDamage(0.5) != 0.5 {
		t.Error("the scribe's speed bonus is wrong")
	}
}

func TestGainXP(t *testing.T) {
	h := NewHero(Knight)
	h.HP, h.MP = 1, 0
	hp, atk := h.MaxHP(), h.ATK()
	if ups := h.GainXP(h.NextXP() - 1); len(ups) != 0 {
		t.Fatalf("levelled up too early: %v", ups)
	}
	ups := h.GainXP(1 + 24) // level 2, then exactly level 3
	if len(ups) != 2 || h.Level != 3 || h.XP != 0 {
		t.Fatalf("got %v, level %d, XP %d", ups, h.Level, h.XP)
	}
	if h.MaxHP() != hp+16 || h.ATK() != atk+4 {
		t.Fatalf("HP %d ATK %d after two levels", h.MaxHP(), h.ATK())
	}
	if ups[0].Gain.DEF != 1 || ups[1].Gain.DEF != 0 {
		t.Fatalf("DEF should grow on even levels only: %v", ups)
	}
	if h.HP != h.MaxHP() || h.MP != h.MaxMP() {
		t.Fatal("a level up does not restore HP and MP")
	}
	if s := ups[0].String(); !strings.Contains(s, "HP +8") || !strings.Contains(s, "DEF +1") {
		t.Fatalf("level up says %q", s)
	}
}

func TestHealAndRestore(t *testing.T) {
	h := NewHero(Scribe)
	h.HP, h.MP = 5, 1
	if n := h.Heal(1000); n != h.MaxHP()-5 || h.HP != h.MaxHP() {
		t.Fatalf("healed %d to %d", n, h.HP)
	}
	if n := h.Restore(2); n != 2 || h.MP != 3 {
		t.Fatalf("restored %d to %d", n, h.MP)
	}
	if h.Hit(1) != 1 {
		t.Fatal("a blow must always do at least 1 damage")
	}
	k := NewHero(Knight)
	if k.Hit(10) != 10-k.DEF() {
		t.Fatalf("DEF does not soften blows: %d", k.Hit(10))
	}
}

func TestGear(t *testing.T) {
	g := Gear{Slot: Weapon, Tier: 0, Base: 0, Affix: Swiftness}
	if g.Name() != "Rusty Quill of Swiftness" {
		t.Fatalf("got %q", g.Name())
	}
	if b := g.Bonus(); b.ATK != 1 || b.Dodge != 10 {
		t.Fatalf("got %+v", b)
	}
	if g.About() != "ATK +1, Dodge +10%" {
		t.Fatalf("got %q", g.About())
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for depth := 1; depth <= 20; depth++ {
		for i := 0; i < 50; i++ {
			g := RandomGear(depth, rng)
			if g.Tier < 0 || g.Tier > MaxTier || g.Tier > (depth+1)/2 {
				t.Fatalf("depth %d: tier %d", depth, g.Tier)
			}
			if g.Slot == Trinket && g.Affix == NoAffix {
				t.Fatal("a trinket with no magic")
			}
			if g.Name() == "" || g.About() == "" || g.SellPrice() <= 0 || g.SellPrice() >= g.Price() {
				t.Fatalf("bad gear %+v: %q %q %d", g, g.Name(), g.About(), g.Price())
			}
		}
	}
}

func TestEquip(t *testing.T) {
	h := NewHero(Rogue)
	old := *h.Gear[Weapon]
	sword := Gear{Slot: Weapon, Tier: 3, Base: 2}
	vest := Gear{Slot: Armor, Tier: 1, Affix: Vigor}
	if !h.Take(vest) || h.Gear[Armor] == nil || len(h.Bag) != 0 {
		t.Fatal("new armour was not worn")
	}
	if !h.Take(sword) || len(h.Bag) != 1 {
		t.Fatal("a second weapon did not go in the bag")
	}
	atk := h.ATK()
	h.Equip(0)
	if *h.Gear[Weapon] != sword || len(h.Bag) != 1 || h.Bag[0] != old {
		t.Fatalf("equip swapped wrongly: %+v, bag %+v", h.Gear[Weapon], h.Bag)
	}
	if h.ATK() <= atk {
		t.Fatal("a better sword did not raise ATK")
	}
	h.HP = h.MaxHP()
	h.Bag = append(h.Bag, Gear{Slot: Armor})
	h.Equip(1) // worse armour, with no Vigor
	if h.HP > h.MaxHP() {
		t.Fatal("HP is above its maximum after changing armour")
	}
	for len(h.Bag) < BagSize {
		h.Bag = append(h.Bag, sword)
	}
	if h.Take(sword) {
		t.Fatal("took gear into a full bag")
	}
	h.Drop(0)
	if len(h.Bag) != BagSize-1 {
		t.Fatal("drop did not remove the item")
	}
}

// A hero must survive a trip through JSON, and Clone must not share gear.
func TestHeroJSON(t *testing.T) {
	h := NewHero(Scribe)
	h.Take(Gear{Slot: Trinket, Tier: 2, Affix: Owl})
	h.Bag = append(h.Bag, Gear{Slot: Armor, Tier: 1})
	h.Items[Clarity] = 2
	h.Hourglass = true
	data, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	var got Hero
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, h) {
		t.Fatalf("got %+v, want %+v", got, h)
	}
	c := h.Clone()
	c.Gear[Weapon].Tier = 5
	c.Bag[0].Tier = 5
	if h.Gear[Weapon].Tier == 5 || h.Bag[0].Tier == 5 {
		t.Fatal("clone shares gear")
	}
}
