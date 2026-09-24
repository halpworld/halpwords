package rpg

import (
	"fmt"
	"math/rand/v2"
)

// Item is a kind of consumable the hero carries.
type Item uint8

const (
	Potion     Item = iota // restores HP
	Ether                  // restores MP
	HintScroll             // shows the next letter, instead of MP
	Hourglass              // more time to type for one battle
	Clarity                // Rune of Clarity: live typo highlighting for one battle

	NumItems
)

// ItemInfo describes a kind of item.
type ItemInfo struct {
	Name, Plural string
	Price        int
	About        string
}

var itemInfo = [NumItems]ItemInfo{
	Potion:     {"Potion", "Potions", 15, "Restores half your HP."},
	Ether:      {"Ether", "Ethers", 20, "Restores half your MP."},
	HintScroll: {"Hint Scroll", "Hint Scrolls", 10, "Shows a letter when you have no MP for a hint."},
	Hourglass:  {"Hourglass", "Hourglasses", 30, "Half as much time again to type, for one battle."},
	Clarity:    {"Rune of Clarity", "Runes of Clarity", 25, "Shows typos as you type, for one battle."},
}

// Info returns the item's description.
func (it Item) Info() *ItemInfo { return &itemInfo[it%NumItems] }

func (it Item) String() string { return it.Info().Name }

// Count says how many items there are, such as "1 Potion" or "2 Ethers".
func (it Item) Count(n int) string {
	if n == 1 {
		return "1 " + it.Info().Name
	}
	return fmt.Sprintf("%d %s", n, it.Info().Plural)
}

// Slot is where a piece of gear is worn.
type Slot uint8

const (
	Weapon Slot = iota
	Armor
	Trinket

	NumSlots
)

func (s Slot) String() string { return [...]string{"Weapon", "Armour", "Trinket"}[s%NumSlots] }

// Affix is the magic on a piece of gear, shown as "of ..." after its name.
type Affix uint8

const (
	NoAffix    Affix = iota
	Swiftness        // more time to dodge
	FocusAffix       // more Focus
	Fortune          // more Luck
	Might            // more ATK
	Warding          // more DEF
	Vigor            // more HP
	Owl              // more MP

	numAffixes
)

var affixNames = [numAffixes]string{"", "of Swiftness", "of Focus", "of Fortune", "of Might", "of Warding", "of Vigor", "of the Owl"}

// Materials name the gear tiers, weakest first.
var Materials = []string{"Rusty", "Iron", "Steel", "Silver", "Runed", "Starforged"}

// MaxTier is the best gear tier.
var MaxTier = len(Materials) - 1

var bases = [NumSlots][]string{
	Weapon:  {"Quill", "Dagger", "Sword", "Staff", "Mace"},
	Armor:   {"Robe", "Jerkin", "Mail", "Cuirass"},
	Trinket: {"Ring", "Amulet", "Charm", "Brooch"},
}

// Gear is a piece of equipment. Its name and powers follow from its parts,
// so it saves as a few numbers.
type Gear struct {
	Slot  Slot
	Tier  int // index into Materials
	Base  int // which kind of weapon, armour or trinket
	Affix Affix
}

// Name returns the gear's name, such as "Rusty Quill of Swiftness".
func (g Gear) Name() string {
	b := bases[g.Slot%NumSlots]
	s := Materials[max(0, min(g.Tier, MaxTier))] + " " + b[g.Base%len(b)]
	if g.Affix != NoAffix && g.Affix < numAffixes {
		s += " " + affixNames[g.Affix]
	}
	return s
}

// Bonus returns what the gear adds to the hero's stats.
func (g Gear) Bonus() Stats {
	var s Stats
	t := max(0, min(g.Tier, MaxTier))
	switch g.Slot {
	case Weapon:
		s.ATK = 1 + 2*t
	case Armor:
		s.DEF = 1 + t
		s.MaxHP = 2 * t
	}
	k := 1 + t/2 // magic grows with the tier
	switch g.Affix {
	case Swiftness:
		s.Dodge += 10 * k
	case FocusAffix:
		s.Focus += k
	case Fortune:
		s.Luck += 2 * k
	case Might:
		s.ATK += 2 * k
	case Warding:
		s.DEF += k
	case Vigor:
		s.MaxHP += 6 * k
	case Owl:
		s.MaxMP += 3 * k
	}
	return s
}

// About describes the gear's powers, such as "ATK +3, Dodge +10%".
func (g Gear) About() string { return describe(g.Bonus()) }

func describe(s Stats) string {
	out := ""
	add := func(n int, f string) {
		if n == 0 {
			return
		}
		if out != "" {
			out += ", "
		}
		out += fmt.Sprintf(f, n)
	}
	add(s.ATK, "ATK +%d")
	add(s.DEF, "DEF +%d")
	add(s.MaxHP, "HP +%d")
	add(s.MaxMP, "MP +%d")
	add(s.Focus, "Focus +%d")
	add(s.Luck, "Luck +%d")
	add(s.Dodge, "Dodge +%d%%")
	return out
}

// Price is what a merchant asks for the gear. They buy it back for half.
func (g Gear) Price() int {
	p := 25 + 30*g.Tier
	if g.Slot == Trinket {
		p += 10
	}
	if g.Affix != NoAffix {
		p += 20 + 10*g.Tier
	}
	return p
}

// SellPrice is what a merchant pays for the gear.
func (g Gear) SellPrice() int { return g.Price() / 2 }

// StarterWeapon is the weapon a new hero of class c starts with.
func StarterWeapon(c Class) Gear {
	base := 2 // sword
	switch c {
	case Scribe:
		base = 0 // quill
	case Rogue:
		base = 1 // dagger
	}
	return Gear{Slot: Weapon, Base: base}
}

// GearTier picks a tier for gear found on floor depth: about one tier for
// every two floors, give or take one.
func GearTier(depth int, rng *rand.Rand) int {
	return max(0, min(MaxTier, (depth-1)/2+rng.IntN(3)-1))
}

// RandomGear makes a piece of gear for floor depth. Trinkets always have
// magic; weapons and armour have it half the time.
func RandomGear(depth int, rng *rand.Rand) Gear {
	g := Gear{Slot: Slot(rng.IntN(int(NumSlots))), Tier: GearTier(depth, rng)}
	g.Base = rng.IntN(len(bases[g.Slot]))
	if g.Slot == Trinket || rng.IntN(2) == 0 {
		g.Affix = Affix(1 + rng.IntN(int(numAffixes)-1))
	}
	return g
}
