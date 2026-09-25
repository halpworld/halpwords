// Package rpg holds the hero's side of the game: classes, stats, levels,
// items and equipment. It has no Ebitengine dependency.
package rpg

import "fmt"

// Class is the kind of hero, chosen at the start of an adventure.
type Class uint8

const (
	Knight Class = iota // more HP and DEF
	Scribe              // bonus damage from speed; hints cost less MP
	Rogue               // longer dodge windows; better chest loot
)

// Classes lists every class, in the order they are offered.
var Classes = []Class{Knight, Scribe, Rogue}

// ClassInfo describes a class: its starting stats and how they grow.
type ClassInfo struct {
	Name  string
	Blurb []string // a few short lines for the class picker
	Start Stats
	Grow  Stats // added at every level; DEF, Focus and Luck grow every other level
}

var classInfo = [...]ClassInfo{
	Knight: {
		Name:  "Knight",
		Blurb: []string{"Tough and armoured.", "The most HP and DEF."},
		Start: Stats{MaxHP: 36, MaxMP: 4, ATK: 6, DEF: 1, Focus: 0, Luck: 1},
		Grow:  Stats{MaxHP: 8, MaxMP: 1, ATK: 2, DEF: 1},
	},
	Scribe: {
		Name:  "Scribe",
		Blurb: []string{"A master of words.", "More damage when fast.", "Hints cost only 1 MP."},
		Start: Stats{MaxHP: 30, MaxMP: 10, ATK: 6, DEF: 0, Focus: 2, Luck: 1},
		Grow:  Stats{MaxHP: 6, MaxMP: 2, ATK: 2, DEF: 1, Focus: 1},
	},
	Rogue: {
		Name:  "Rogue",
		Blurb: []string{"Quick and lucky.", "25% more time to dodge.", "+50% gold from chests."},
		Start: Stats{MaxHP: 30, MaxMP: 6, ATK: 6, DEF: 0, Focus: 1, Luck: 4},
		Grow:  Stats{MaxHP: 6, MaxMP: 1, ATK: 2, DEF: 1, Luck: 1},
	},
}

// Info returns the class's description.
func (c Class) Info() *ClassInfo {
	if int(c) >= len(classInfo) {
		c = Knight
	}
	return &classInfo[c]
}

func (c Class) String() string { return c.Info().Name }

// Stats are the numbers that decide how the hero fights.
type Stats struct {
	MaxHP, MaxMP int
	ATK          int // attack power
	DEF          int // taken off every blow a monster lands
	Focus        int // each point gives 5% more time to type
	Luck         int // each point gives 2% more critical hits and 5% more gold
	Dodge        int // extra time to dodge, in percent
}

// Add returns s plus t.
func (s Stats) Add(t Stats) Stats {
	return Stats{
		MaxHP: s.MaxHP + t.MaxHP, MaxMP: s.MaxMP + t.MaxMP,
		ATK: s.ATK + t.ATK, DEF: s.DEF + t.DEF,
		Focus: s.Focus + t.Focus, Luck: s.Luck + t.Luck, Dodge: s.Dodge + t.Dodge,
	}
}

// Hero is the player character. It is saved as JSON, so every field that
// matters is exported.
type Hero struct {
	Class     Class
	Level, XP int
	HP, MP    int
	Base      Stats // the class's stats at this level, before gear
	Gold      int
	Items     [NumItems]int // how many of each item the hero carries
	Gear      [NumSlots]*Gear
	Bag       []Gear // gear that is carried but not worn
	Streak    int    // good answers in a row, for the combo bonus
	// Hourglass and Clarity are armed for the next battle.
	Hourglass, Clarity bool
}

// BagSize is how many pieces of spare gear the hero can carry.
const BagSize = 6

// NewHero returns a level 1 hero of class c with a potion and a rusty
// weapon.
func NewHero(c Class) Hero {
	info := c.Info()
	h := Hero{Class: c, Level: 1, Base: info.Start}
	h.Items[Potion] = 1
	w := StarterWeapon(c)
	h.Gear[Weapon] = &w
	h.HP, h.MP = h.MaxHP(), h.MaxMP()
	return h
}

// Clone returns a copy of h that shares nothing with it.
func (h Hero) Clone() Hero {
	for i, g := range h.Gear {
		if g != nil {
			c := *g
			h.Gear[i] = &c
		}
	}
	h.Bag = append([]Gear(nil), h.Bag...)
	return h
}

// Stats returns the hero's stats with their gear.
func (h *Hero) Stats() Stats {
	s := h.Base
	for _, g := range h.Gear {
		if g != nil {
			s = s.Add(g.Bonus())
		}
	}
	return s
}

func (h *Hero) MaxHP() int { return h.Stats().MaxHP }
func (h *Hero) MaxMP() int { return h.Stats().MaxMP }
func (h *Hero) ATK() int   { return h.Stats().ATK }
func (h *Hero) DEF() int   { return h.Stats().DEF }

// PerfectXP is the XP for the first perfect spelling of a word in an
// adventure.
const PerfectXP = 2

// PuzzleXP is the XP for solving the puzzle on a chest, or else a sealed
// door, on floor depth.
func PuzzleXP(chest bool, depth int) int {
	if chest {
		return 3 + depth/2
	}
	return 2 + depth/2
}

// NextXP is the experience needed for the next level.
func (h *Hero) NextXP() int { return 8*h.Level + 4*h.Level*h.Level }

// LevelUp is what one level gained.
type LevelUp struct {
	Level int
	Gain  Stats
}

func (u LevelUp) String() string {
	s := fmt.Sprintf("HP +%d, MP +%d, ATK +%d", u.Gain.MaxHP, u.Gain.MaxMP, u.Gain.ATK)
	for _, x := range []struct {
		n    int
		name string
	}{{u.Gain.DEF, "DEF"}, {u.Gain.Focus, "Focus"}, {u.Gain.Luck, "Luck"}} {
		if x.n > 0 {
			s += fmt.Sprintf(", %s +%d", x.name, x.n)
		}
	}
	return s
}

// GainXP adds experience and returns the levels gained. Each level raises
// the class's stats and restores HP and MP.
func (h *Hero) GainXP(xp int) []LevelUp {
	h.XP += xp
	var ups []LevelUp
	for h.XP >= h.NextXP() {
		h.XP -= h.NextXP()
		h.Level++
		g := h.Class.Info().Grow
		if h.Level%2 == 1 {
			// DEF, Focus and Luck grow every other level.
			g.DEF, g.Focus, g.Luck = 0, 0, 0
		}
		h.Base = h.Base.Add(g)
		h.HP, h.MP = h.MaxHP(), h.MaxMP()
		ups = append(ups, LevelUp{h.Level, g})
	}
	return ups
}

// Heal restores up to n HP and returns how much it restored.
func (h *Hero) Heal(n int) int {
	n = max(0, min(n, h.MaxHP()-h.HP))
	h.HP += n
	return n
}

// Restore gives back up to n MP and returns how much it gave.
func (h *Hero) Restore(n int) int {
	n = max(0, min(n, h.MaxMP()-h.MP))
	h.MP += n
	return n
}

// PotionHeal is how much a potion restores.
func (h *Hero) PotionHeal() int { return max(12, h.MaxHP()/2) }

// EtherRestore is how much MP an ether restores.
func (h *Hero) EtherRestore() int { return max(4, h.MaxMP()/2) }

// HintCost is the MP a hint costs.
func (h *Hero) HintCost() int {
	if h.Class == Scribe {
		return 1
	}
	return 2
}

// TimeBonus is how much longer than usual the hero has to type: Focus,
// the Rogue's quick reflexes and gear all add to it.
func (h *Hero) TimeBonus() float64 {
	s := h.Stats()
	b := 1 + 0.05*float64(s.Focus) + float64(s.Dodge)/100
	return b
}

// DodgeBonus is the extra time the hero has to dodge.
func (h *Hero) DodgeBonus() float64 {
	b := h.TimeBonus()
	if h.Class == Rogue {
		b += 0.25
	}
	return b
}

// LuckyCrit is the chance that a good hit that is not already critical
// becomes one.
func (h *Hero) LuckyCrit() float64 { return min(0.3, 0.02*float64(h.Stats().Luck)) }

// GoldFind scales gold found in chests and on monsters.
func (h *Hero) GoldFind(gold int, chest bool) int {
	k := 1 + 0.05*float64(h.Stats().Luck)
	if chest && h.Class == Rogue {
		k += 0.5
	}
	return int(float64(gold)*k + 0.5)
}

// SpeedDamage returns the speed multiplier used for damage. Scribes get
// half as much again from anything above normal speed.
func (h *Hero) SpeedDamage(speed float64) float64 {
	if h.Class == Scribe && speed > 1 {
		return 1 + (speed-1)*1.5
	}
	return speed
}

// Hit returns the damage a monster's blow of n does after DEF. However
// strong the armour, a blow always does a third of its power, and at
// least 1.
func (h *Hero) Hit(n int) int { return max(1, (n+2)/3, n-h.DEF()) }

// Equip wears bag item i, putting whatever was in its slot into the bag.
func (h *Hero) Equip(i int) {
	if i < 0 || i >= len(h.Bag) {
		return
	}
	g := h.Bag[i]
	h.Bag = append(h.Bag[:i], h.Bag[i+1:]...)
	if old := h.Gear[g.Slot]; old != nil {
		h.Bag = append(h.Bag, *old)
	}
	h.Gear[g.Slot] = &g
	h.clamp()
}

// Take puts new gear on if its slot is empty, or else in the bag. It
// reports false when the bag is full.
func (h *Hero) Take(g Gear) bool {
	if h.Gear[g.Slot] == nil {
		h.Gear[g.Slot] = &g
		return true
	}
	if len(h.Bag) >= BagSize {
		return false
	}
	h.Bag = append(h.Bag, g)
	return true
}

// Drop throws away bag item i.
func (h *Hero) Drop(i int) {
	if i >= 0 && i < len(h.Bag) {
		h.Bag = append(h.Bag[:i], h.Bag[i+1:]...)
	}
}

// clamp keeps HP and MP within their maximums after gear changes.
func (h *Hero) clamp() {
	h.HP = min(h.HP, h.MaxHP())
	h.MP = min(h.MP, h.MaxMP())
}
