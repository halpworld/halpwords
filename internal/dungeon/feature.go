package dungeon

import (
	"math/rand/v2"

	"github.com/halpworld/halpwords/internal/rpg"
)

// FeatureKind is something in a room the hero can use: a shrine, a
// campfire or a merchant. Features block movement like chests do.
type FeatureKind uint8

const (
	Shrine   FeatureKind = iota // a Save Shrine
	Campfire                    // rest to restore HP and MP, once
	Merchant                    // buys and sells
)

func (k FeatureKind) String() string {
	return [...]string{"Save Shrine", "Campfire", "Merchant"}[k%3]
}

// Feature is a shrine, campfire or merchant on a floor.
type Feature struct {
	Kind  FeatureKind
	Used  bool       // a campfire that has burnt down
	Stock []rpg.Gear `json:",omitempty"` // a merchant's gear for sale
}

// BossFloor reports whether a boss guards the stairs on floor depth.
func BossFloor(depth int) bool { return depth%3 == 0 }

// ShrineFloor reports whether floor depth has a Save Shrine: two floors in
// every three, always including the boss floors, but never the first.
func ShrineFloor(depth int) bool { return depth > 1 && depth%3 != 1 }

// StockSize is how many pieces of gear a merchant sells.
const StockSize = 3

// placeFeatures puts a shrine in the start room on shrine floors, and on
// some floors a campfire and a merchant in rooms of their own. Each room
// holds at most one thing that blocks the way, so none can cut it in two.
func (f *Level) placeFeatures(rng *rand.Rand) {
	if ShrineFloor(f.Depth) {
		if cells := f.quietCells(f.Rooms[0]); len(cells) > 0 {
			f.Features[cells[rng.IntN(len(cells))]] = &Feature{Kind: Shrine}
		}
	}
	var free []int // rooms with nothing in them, not the start or the stairs
	for i, r := range f.Rooms {
		if i == 0 || r.Contains(f.Exit) || f.roomHasBlocker(r) {
			continue
		}
		free = append(free, i)
	}
	rng.Shuffle(len(free), func(i, j int) { free[i], free[j] = free[j], free[i] })
	put := func(k FeatureKind) *Feature {
		for n, i := range free {
			cells := f.quietCells(f.Rooms[i])
			if len(cells) == 0 {
				continue
			}
			free = append(free[:n], free[n+1:]...)
			ft := &Feature{Kind: k}
			f.Features[cells[rng.IntN(len(cells))]] = ft
			return ft
		}
		return nil
	}
	if f.Depth >= 2 && (BossFloor(f.Depth) || rng.IntN(2) == 0) {
		put(Campfire)
	}
	if f.Depth == 2 || (f.Depth > 2 && rng.IntN(2) == 0) {
		if m := put(Merchant); m != nil {
			for i := 0; i < StockSize; i++ {
				m.Stock = append(m.Stock, rpg.RandomGear(f.Depth+1, rng))
			}
		}
	}
}

// roomHasBlocker reports whether a chest or feature stands in room r.
func (f *Level) roomHasBlocker(r Room) bool {
	for p := range f.Chests {
		if r.Contains(p) {
			return true
		}
	}
	for p := range f.Features {
		if r.Contains(p) {
			return true
		}
	}
	return false
}

// FeatureAt returns the feature at p, or nil.
func (f *Level) FeatureAt(p Point) *Feature { return f.Features[p] }

// Boss returns the living boss on the floor, or nil.
func (f *Level) Boss() *Monster {
	for _, m := range f.Monsters {
		if m.Kind.Boss() && m.HP > 0 {
			return m
		}
	}
	return nil
}

// placeBoss puts the floor's boss in the stairs room, next to the stairs
// when there is space. The stairs do not work until it is defeated.
func (f *Level) placeBoss(rng *rand.Rand) {
	if !BossFloor(f.Depth) {
		return
	}
	var room Room
	for _, r := range f.Rooms {
		if r.Contains(f.Exit) {
			room = r
		}
	}
	var near, rest []Point
	for y := room.Y; y < room.Y+room.H; y++ {
		for x := room.X; x < room.X+room.W; x++ {
			p := Point{x, y}
			if f.At(p) != Floor || f.Blocked(p) {
				continue
			}
			if p.Manhattan(f.Exit) == 1 {
				near = append(near, p)
			} else {
				rest = append(rest, p)
			}
		}
	}
	cells := near
	if len(cells) == 0 {
		cells = rest
	}
	if len(cells) == 0 {
		return
	}
	p := cells[rng.IntN(len(cells))]
	m := NewMonster(BossFor(f.Depth), f.Depth, p, rng.Uint64())
	if d, ok := dirBetween(p, f.Exit); ok {
		m.Facing = d.Back() // guarding: back to the stairs
	}
	f.Monsters = append(f.Monsters, m)
}

func dirBetween(a, b Point) (Dir, bool) {
	for d := North; d <= West; d++ {
		if a.Step(d) == b {
			return d, true
		}
	}
	return 0, false
}
