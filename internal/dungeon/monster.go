package dungeon

import "math/rand/v2"

// Family decides how a monster's sprite is generated and how it moves.
type Family uint8

const (
	Slime Family = iota
	Bat
	Skull
	Ghost
	Eye
	Spider
	Golem
	Imp
	Rat
)

// Kind is a type of monster.
type Kind struct {
	Name   string
	Family Family
	HP     int // at depth 1
	ATK    int // at depth 1
	XP     int
	Gold   int
	MinDep int // first floor it appears on
	Hue    int // which colour ramp the sprite uses
	Size   float64
}

// Kinds lists every monster, roughly weakest first.
var Kinds = []Kind{
	{"Green Slime", Slime, 10, 3, 4, 3, 1, 0, 0.55},
	{"Cave Bat", Bat, 8, 3, 4, 2, 1, 1, 0.45},
	{"Grumpy Rat", Rat, 11, 4, 5, 3, 1, 2, 0.5},
	{"Bone Rattler", Skull, 14, 4, 7, 5, 2, 3, 0.6},
	{"Wisp", Ghost, 12, 5, 7, 4, 2, 4, 0.6},
	{"Gazer", Eye, 16, 5, 9, 6, 3, 5, 0.6},
	{"Crypt Spider", Spider, 15, 6, 9, 6, 3, 6, 0.6},
	{"Blue Slime", Slime, 20, 6, 10, 7, 4, 7, 0.6},
	{"Moss Golem", Golem, 26, 7, 13, 9, 5, 8, 0.8},
	{"Fire Imp", Imp, 22, 8, 13, 9, 5, 9, 0.6},
}

// RandomKind picks a monster that can appear at depth.
func RandomKind(depth int, rng *rand.Rand) *Kind {
	var pool []*Kind
	for i := range Kinds {
		if Kinds[i].MinDep <= depth {
			pool = append(pool, &Kinds[i])
		}
	}
	// Favour the newest monsters a little.
	if len(pool) > 3 && rng.IntN(2) == 0 {
		pool = pool[len(pool)-3:]
	}
	return pool[rng.IntN(len(pool))]
}

// Monster is one monster on a floor.
type Monster struct {
	Kind   *Kind
	At     Point
	HP     int
	MaxHP  int
	ATK    int
	Seed   uint64 // sprite seed
	Awake  bool   // has noticed the hero
	Stun   int    // turns left before it moves again
	Facing Dir
}

// NewMonster creates a monster of kind k, with stats scaled for depth.
func NewMonster(k *Kind, depth int, at Point, seed uint64) *Monster {
	grow := 1 + 0.15*float64(depth-1)
	hp := int(float64(k.HP) * grow)
	return &Monster{Kind: k, At: at, HP: hp, MaxHP: hp, ATK: int(float64(k.ATK) * grow), Seed: seed}
}

// Name returns the monster's display name.
func (m *Monster) Name() string { return m.Kind.Name }

// XP and Gold are the rewards for defeating the monster.
func (m *Monster) XP() int   { return m.Kind.XP + m.MaxHP/5 }
func (m *Monster) Gold() int { return m.Kind.Gold }

// SightRange is how far away (in steps) a monster notices the hero.
const SightRange = 6

// MoveMonsters gives every monster one turn after the hero acts. Awake
// monsters walk towards the hero; others wander. It returns the first monster
// that ends up next to the hero, which then starts a battle.
func (f *Level) MoveMonsters(hero Point, rng *rand.Rand) *Monster {
	dist := f.distances(hero, false)
	var attacker *Monster
	for _, m := range f.Monsters {
		if m.HP <= 0 {
			continue
		}
		if m.Stun > 0 {
			m.Stun--
			continue
		}
		d := dist[f.Index(m.At)]
		if d >= 0 && d <= SightRange {
			m.Awake = true
		} else if d < 0 || d > SightRange*2 {
			m.Awake = false
		}
		if m.At.Manhattan(hero) == 1 {
			if attacker == nil {
				attacker = m
			}
			continue
		}
		if m.Awake && d > 0 {
			// Step to a neighbour that is closer to the hero.
			for _, dir := range rng.Perm(4) {
				q := m.At.Step(Dir(dir))
				if f.In(q) && dist[f.Index(q)] >= 0 && dist[f.Index(q)] < d && q != hero && !f.Blocked(q) {
					m.At, m.Facing = q, Dir(dir)
					break
				}
			}
		} else if rng.IntN(3) == 0 {
			dir := Dir(rng.IntN(4))
			q := m.At.Step(dir)
			if q != hero && !f.Blocked(q) && f.At(q) != Stairs {
				m.At, m.Facing = q, dir
			}
		}
		if m.At.Manhattan(hero) == 1 && attacker == nil {
			attacker = m
		}
	}
	return attacker
}
