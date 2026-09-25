// Package sim plays the dungeon without a screen, with bots that type at
// a set skill, to check the game's balance: how long fights take, how much
// they hurt, and how often a hero falls on each floor. It uses the same
// dungeon, monsters, hero and formulas as the game, and has no Ebitengine
// dependency.
package sim

import (
	"cmp"
	"maps"
	"math"
	"math/rand/v2"
	"slices"
	"unicode/utf8"

	"github.com/halpworld/halpwords/internal/combat"
	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/internal/words"
)

// Typist is how well a bot spells and how fast it types.
type Typist struct {
	Name string
	// Perfect, Correct, Slip and Graze are the shares of answers at each
	// grade for a five-letter word; the rest are misses. Longer words
	// have more mistakes.
	Perfect, Correct, Slip, Graze float64
	// Start and PerChar give the typing time: Start seconds to read the
	// word, then PerChar for each character.
	Start, PerChar float64
}

// Typists are the skills the balance is checked for.
var Typists = []Typist{
	{Name: "beginner", Perfect: 0.40, Correct: 0.15, Slip: 0.12, Graze: 0.10, Start: 1.6, PerChar: 0.55},
	{Name: "average", Perfect: 0.58, Correct: 0.14, Slip: 0.09, Graze: 0.07, Start: 1.2, PerChar: 0.38},
	{Name: "strong", Perfect: 0.78, Correct: 0.10, Slip: 0.05, Graze: 0.03, Start: 0.8, PerChar: 0.24},
}

// answer is one answer typed by the bot.
type answer struct {
	tier words.Tier
	secs float64
}

// answer spells a word of n characters. harder makes mistakes more likely,
// for monsters that hide or mirror their words.
func (t *Typist) answer(n int, harder bool, rng *rand.Rand) answer {
	// Mistakes grow by a sixth for each character over five.
	k := max(0.5, 1+float64(n-5)/6)
	if harder {
		k *= 1.3
	}
	good := t.Perfect / k
	shares := []float64{good, t.Correct, t.Slip * k, t.Graze * k}
	tiers := []words.Tier{words.Perfect, words.Correct, words.AccentSlip, words.Graze}
	r := rng.Float64()
	tier := words.Miss
	for i, s := range shares {
		if r < s {
			tier = tiers[i]
			break
		}
		r -= s
	}
	// Typing times vary by about a quarter either way.
	secs := (t.Start + t.PerChar*float64(n)) * math.Exp(rng.NormFloat64()*0.25)
	return answer{tier, secs}
}

// FloorStats is what happened on one floor of one run.
type FloorStats struct {
	Depth   int
	Fights  int
	Turns   int     // the hero's attacks, over every fight
	HPLost  float64 // HP lost in fights, as a share of the hero's HP
	Falls   int     // times the hero fell
	Potions int     // potions drunk
	Level   int     // the hero's level when they left the floor
	BossWon bool    // a boss floor's boss was beaten without falling
	BossHP  float64 // HP lost to the boss, as a share of the hero's HP
	Gold    int
}

// Run plays floors 1 to floors as a hero of class, typing like t, through
// the dungeon made from seed. entries are the words monsters ask for. A
// hero who falls gets up where they fell with full HP, so every floor is
// played and the falls counted.
func Run(t Typist, class rpg.Class, entries []words.Entry, seed uint64, floors int) []FloorStats {
	rng := rand.New(rand.NewPCG(seed, 0x51ab1e))
	b := &bot{t: t, rng: rng, hero: rpg.NewHero(class), deck: words.NewDeck(entries, rng), perfect: map[int]bool{}}
	var out []FloorStats
	for depth := 1; depth <= floors; depth++ {
		out = append(out, b.floor(seed*31+uint64(depth)*7919, depth))
	}
	return out
}

// bot is a hero played by a Typist.
type bot struct {
	t       Typist
	rng     *rand.Rand
	hero    rpg.Hero
	deck    *words.Deck
	perfect map[int]bool
	st      *FloorStats
	depth   int
}

// floor plays one floor: every monster, every chest and sealed door, a
// rest at the campfire when it is most needed, and the boss last.
func (b *bot) floor(seed uint64, depth int) FloorStats {
	l := dungeon.Generate(seed, depth)
	b.st, b.depth = &FloorStats{Depth: depth}, depth
	h := &b.hero

	boss := l.Boss()
	for _, m := range l.Monsters {
		if m != boss {
			b.fight(m, b.rng.IntN(2) == 0)
		}
	}
	for y := 0; y < l.H; y++ {
		for x := 0; x < l.W; x++ {
			if l.At(dungeon.Point{X: x, Y: y}) == dungeon.Sealed {
				b.puzzle(nil)
			}
		}
	}
	// Maps are visited in order, so a seed always plays the same.
	for _, p := range sorted(l.Chests) {
		b.puzzle(l.Chests[p])
	}
	for _, p := range sorted(l.Features) {
		if ft := l.Features[p]; ft.Kind == dungeon.Merchant {
			b.shop(ft.Stock)
		}
	}
	if boss != nil {
		for _, ft := range l.Features {
			if ft.Kind == dungeon.Campfire {
				h.Heal(h.MaxHP())
			}
		}
		falls, lost := b.st.Falls, b.st.HPLost
		b.fight(boss, false)
		b.st.BossWon = b.st.Falls == falls
		b.st.BossHP = b.st.HPLost - lost
	}
	b.st.Level, b.st.Gold = h.Level, h.Gold
	return *b.st
}

// sorted returns the cells of a map, top to bottom and left to right.
func sorted[V any](m map[dungeon.Point]V) []dungeon.Point {
	ps := slices.Collect(maps.Keys(m))
	slices.SortFunc(ps, func(a, b dungeon.Point) int { return cmp.Or(a.Y-b.Y, a.X-b.X) })
	return ps
}

// shop buys gear that is better than what is worn, then potions, up to
// three.
func (b *bot) shop(stock []rpg.Gear) {
	h := &b.hero
	for _, g := range stock {
		if h.Gold >= g.Price() && better(g, h.Gear[g.Slot]) {
			h.Gold -= g.Price()
			h.Gear[g.Slot] = &g
		}
	}
	price := rpg.Potion.Info().Price
	for h.Items[rpg.Potion] < 3 && h.Gold >= price {
		h.Gold -= price
		h.Items[rpg.Potion]++
	}
}

// fall is the hero dropping: counted, then back up with full HP.
func (b *bot) fall() {
	b.st.Falls++
	b.hero.HP = b.hero.MaxHP()
	b.hero.Streak = 0
}

// word deals a word for a monster, as the game does.
func (b *bot) word(m *dungeon.Monster) (words.Entry, int) {
	target := combat.WordTarget(b.depth, m.Kind.Boss())
	if m.Phase > 0 {
		if e, id, ok := b.deck.NextNear(target, func(e words.Entry) bool { return utf8.RuneCountInString(e.Answers[0]) >= 6 }); ok {
			return e, id
		}
	}
	e, id, _ := b.deck.NextNear(target, nil)
	return e, id
}

// score records an answer, as the game does, for the streak, the deck
// and first perfect spellings.
func (b *bot) score(id int, a answer) {
	h := &b.hero
	b.deck.Answer(id, words.Answer{Tier: a.tier, Secs: a.secs, Timed: true})
	switch {
	case a.tier >= words.Correct:
		h.Streak++
	case a.tier < words.AccentSlip:
		h.Streak = 0
	}
	if a.tier == words.Perfect && !b.perfect[id] {
		b.perfect[id] = true
		h.GainXP(rpg.PerfectXP)
	}
}

// fight battles m until one of them drops. After an ambush the monster
// strikes first.
func (b *bot) fight(m *dungeon.Monster, ambush bool) {
	h := &b.hero
	b.st.Fights++
	lost := 0
	defend := ambush
	for m.HP > 0 {
		if !defend {
			if h.HP*100 < h.MaxHP()*35 && h.Items[rpg.Potion] > 0 {
				h.Items[rpg.Potion]--
				h.Heal(h.PotionHeal())
				b.st.Potions++
			} else {
				b.strike(m)
				if m.HP <= 0 {
					break
				}
			}
		}
		defend = false
		hp := h.HP
		b.dodge(m)
		lost += hp - max(0, h.HP)
		if h.HP <= 0 {
			b.fall()
		}
	}
	b.st.HPLost += float64(lost) / float64(max(1, h.MaxHP()))
	h.Gold += h.GoldFind(m.Gold(), false)
	h.GainXP(m.XP())
}

// strike is the hero's attack, as in the game.
func (b *bot) strike(m *dungeon.Monster) {
	h := &b.hero
	b.st.Turns++
	e, id := b.word(m)
	n := utf8.RuneCountInString(e.Answers[0])
	a := b.t.answer(n, m.Has(dungeon.Ghostly) || m.Has(dungeon.Mirrored), b.rng)
	speed := combat.Speed(n, a.secs/h.TimeBonus())
	dmg, crit := combat.Damage(h.ATK(), a.tier, h.SpeedDamage(speed), h.Streak)
	if !crit && a.tier >= words.Correct && b.rng.Float64() < h.LuckyCrit() {
		dmg = int(float64(dmg)*1.5 + 0.5)
	}
	b.score(id, a)
	switch {
	case a.tier > words.Miss && m.Has(dungeon.Armored) && combat.ArmorBlocks(a.tier):
	case a.tier == words.Miss || dmg == 0:
		h.HP--
	default:
		m.HP -= dmg
		if m.Kind.Boss() && m.HP > 0 {
			for p := combat.BossPhase(m.HP, m.MaxHP); m.Phase < p; {
				m.Phase++
				t := dungeon.Swift
				if m.Phase == 2 {
					t = dungeon.Mirrored
				}
				m.Traits |= t
			}
		}
	}
	if h.HP <= 0 {
		b.fall()
	}
}

// dodge is the monster's attack, as in the game.
func (b *bot) dodge(m *dungeon.Monster) {
	h := &b.hero
	e, id := b.word(m)
	n := utf8.RuneCountInString(e.Answers[0])
	limit := combat.DefendTime(n) * h.DodgeBonus()
	if m.Has(dungeon.Swift) {
		limit *= combat.SwiftTime
	}
	a := b.t.answer(n, m.Has(dungeon.Ghostly) || m.Has(dungeon.Mirrored), b.rng)
	if a.secs > limit {
		a.tier = words.Miss
	}
	b.score(id, a)
	hit := h.Hit(max(1, m.ATK+b.rng.IntN(3)-1))
	h.HP -= int(math.Round(float64(hit) * combat.Block(a.tier)))
}

// puzzle opens a chest, or a sealed door when ch is nil. There is no
// timer, and accent slips are accepted; a wrong answer stings for 2 HP and
// a Mimic chest wakes up, and drops what it guarded when beaten. A new
// puzzle is dealt until one is solved.
func (b *bot) puzzle(ch *dungeon.Chest) {
	h := &b.hero
	for {
		a := b.t.answer(6, false, b.rng)
		if a.tier >= words.AccentSlip {
			break
		}
		if ch != nil && ch.Mimic {
			m := dungeon.NewMonster(&dungeon.MimicKind, b.depth, dungeon.Point{}, 0)
			m.Loot = &dungeon.Chest{Gold: ch.Gold}
			b.fight(m, true)
			h.Items[rpg.Potion] += ch.Potions
			return
		}
		h.HP -= 2
		if h.HP <= 0 {
			b.fall()
		}
	}
	h.GainXP(rpg.PuzzleXP(ch != nil, b.depth))
	if ch == nil {
		return
	}
	h.Gold += h.GoldFind(ch.Gold, true)
	h.Items[rpg.Potion] += ch.Potions
	for _, it := range ch.Items {
		h.Items[it]++
	}
	if g := ch.Gear; g != nil && better(*g, h.Gear[g.Slot]) {
		h.Gear[g.Slot] = g
	}
}

// better reports whether gear g is an upgrade on what is worn.
func better(g rpg.Gear, worn *rpg.Gear) bool {
	if worn == nil {
		return true
	}
	worth := func(s rpg.Stats) int {
		return 3*s.ATK + 3*s.DEF + s.MaxHP/2 + s.Focus + s.Luck + s.Dodge/5
	}
	return worth(g.Bonus()) > worth(worn.Bonus())
}

// Summary is the average of many runs, floor by floor.
type Summary struct {
	Depth     int
	TurnsPer  float64 // the hero's attacks per fight
	HPPer     float64 // share of the hero's HP lost per fight
	Falls     float64 // falls per run on this floor
	FellOnce  float64 // share of runs that fell at least once on this floor
	BossWon   float64 // share of runs that beat the boss without falling
	BossHP    float64 // share of the hero's HP the boss took
	Level     float64
	Potions   float64 // potions drunk per run on this floor
	Gold      float64
	BossFloor bool
}

// Summarize averages runs of the same floors.
func Summarize(runs [][]FloorStats) []Summary {
	if len(runs) == 0 {
		return nil
	}
	out := make([]Summary, len(runs[0]))
	n := float64(len(runs))
	for i := range out {
		s := &out[i]
		s.Depth = i + 1
		s.BossFloor = dungeon.BossFloor(s.Depth)
		fights, turns := 0, 0
		for _, r := range runs {
			f := r[i]
			fights += f.Fights
			turns += f.Turns
			s.HPPer += f.HPLost
			s.Falls += float64(f.Falls) / n
			if f.Falls > 0 {
				s.FellOnce += 1 / n
			}
			if f.BossWon {
				s.BossWon += 1 / n
			}
			s.BossHP += f.BossHP / n
			s.Level += float64(f.Level) / n
			s.Potions += float64(f.Potions) / n
			s.Gold += float64(f.Gold) / n
		}
		s.TurnsPer = float64(turns) / float64(max(1, fights))
		s.HPPer /= float64(max(1, fights))
	}
	return out
}
