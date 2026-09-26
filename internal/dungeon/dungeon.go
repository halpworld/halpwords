// Package dungeon generates dungeon floors on a grid and moves the monsters
// on them. It has no Ebitengine dependency.
package dungeon

import "github.com/halpworld/halpwords/internal/rpg"

// Tile is one grid cell. Walls are whole cells, as in Eye of the Beholder
// or Dungeon Master.
type Tile uint8

const (
	Wall     Tile = iota
	Floor         // open floor
	Door          // closed door; walking into it opens it
	Sealed        // rune-sealed door; opened by spelling a word
	OpenDoor      // an opened door
	Stairs        // stairs down to the next floor
)

// Walkable reports whether the hero can stand on t.
func (t Tile) Walkable() bool { return t == Floor || t == OpenDoor || t == Stairs }

// Solid reports whether t blocks sight.
func (t Tile) Solid() bool { return t == Wall || t == Door || t == Sealed }

// Dir is a compass direction.
type Dir uint8

const (
	North Dir = iota
	East
	South
	West
)

var dirDelta = [4]Point{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}

// Delta returns the one-cell step in direction d.
func (d Dir) Delta() Point { return dirDelta[d&3] }

// Left and Right turn 90°, Back turns 180°.
func (d Dir) Left() Dir  { return (d + 3) & 3 }
func (d Dir) Right() Dir { return (d + 1) & 3 }
func (d Dir) Back() Dir  { return (d + 2) & 3 }

func (d Dir) String() string { return [4]string{"N", "E", "S", "W"}[d&3] }

// Point is a grid position.
type Point struct{ X, Y int }

// Add returns p moved by q.
func (p Point) Add(q Point) Point { return Point{p.X + q.X, p.Y + q.Y} }

// Step returns the neighbour of p in direction d.
func (p Point) Step(d Dir) Point { return p.Add(d.Delta()) }

// Manhattan returns the grid distance between p and q.
func (p Point) Manhattan(q Point) int { return abs(p.X-q.X) + abs(p.Y-q.Y) }

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// Room is a rectangle of floor.
type Room struct{ X, Y, W, H int }

// Contains reports whether p is inside the room.
func (r Room) Contains(p Point) bool {
	return p.X >= r.X && p.X < r.X+r.W && p.Y >= r.Y && p.Y < r.Y+r.H
}

// Center returns the room's middle cell.
func (r Room) Center() Point { return Point{r.X + r.W/2, r.Y + r.H/2} }

// Chest is a treasure chest. It blocks movement; the hero opens it by solving
// a word puzzle while facing it.
type Chest struct {
	Open    bool
	Gold    int
	Potions int
	Items   []rpg.Item `json:",omitempty"` // other things inside
	Gear    *rpg.Gear  `json:",omitempty"`
	// Mimic chests bite: failing their puzzle wakes a monster.
	Mimic bool
}

// Level is one floor of the dungeon.
type Level struct {
	W, H  int
	Depth int // 1 for the first floor
	Seed  uint64

	tiles []Tile
	// Seen marks cells the hero has seen, for the automap.
	Seen []bool
	// Torches marks wall cells that have a torch.
	Torches []bool

	Rooms    []Room
	Start    Point
	StartDir Dir
	Exit     Point // the stairs
	Chests   map[Point]*Chest
	Features map[Point]*Feature
	Monsters []*Monster

	// Locks are the puzzles a hand-made map sets on its sealed doors and
	// chests; the others get random ones. Generated floors have none.
	Locks map[Point]Lock
	// Notes are texts written on walls of a hand-made map, read when the
	// hero faces them.
	Notes map[Point]string
}

// In reports whether p is on the map.
func (f *Level) In(p Point) bool { return p.X >= 0 && p.Y >= 0 && p.X < f.W && p.Y < f.H }

// At returns the tile at p. Cells off the map are walls.
func (f *Level) At(p Point) Tile {
	if !f.In(p) {
		return Wall
	}
	return f.tiles[p.Y*f.W+p.X]
}

// Set changes the tile at p.
func (f *Level) Set(p Point, t Tile) {
	if f.In(p) {
		f.tiles[p.Y*f.W+p.X] = t
	}
}

// Index returns the position of p in per-cell slices such as Seen.
func (f *Level) Index(p Point) int { return p.Y*f.W + p.X }

// MonsterAt returns the living monster at p, or nil.
func (f *Level) MonsterAt(p Point) *Monster {
	for _, m := range f.Monsters {
		if m.At == p && m.HP > 0 {
			return m
		}
	}
	return nil
}

// Blocked reports whether something stops the hero or a monster entering p.
func (f *Level) Blocked(p Point) bool {
	if !f.At(p).Walkable() {
		return true
	}
	if f.Chests[p] != nil || f.Features[p] != nil {
		return true
	}
	return f.MonsterAt(p) != nil
}

// Remove takes a defeated monster off the floor.
func (f *Level) Remove(m *Monster) {
	for i, o := range f.Monsters {
		if o == m {
			f.Monsters = append(f.Monsters[:i], f.Monsters[i+1:]...)
			return
		}
	}
}

// WakeMimic turns the mimic chest at p into a monster that guards the
// chest's loot.
func (f *Level) WakeMimic(p Point, seed uint64) *Monster {
	c := f.Chests[p]
	delete(f.Chests, p)
	m := NewMonster(&MimicKind, f.Depth, p, seed)
	m.Loot, m.Awake = c, true
	f.Monsters = append(f.Monsters, m)
	return m
}

// distances returns the walking distance from p to every cell (-1 when
// unreachable). Closed doors count as passable when doors is true.
func (f *Level) distances(from Point, doors bool) []int {
	dist := make([]int, f.W*f.H)
	for i := range dist {
		dist[i] = -1
	}
	dist[f.Index(from)] = 0
	queue := []Point{from}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for d := North; d <= West; d++ {
			q := p.Step(d)
			if !f.In(q) || dist[f.Index(q)] >= 0 {
				continue
			}
			t := f.At(q)
			if !t.Walkable() && !(doors && (t == Door || t == Sealed)) {
				continue
			}
			dist[f.Index(q)] = dist[f.Index(p)] + 1
			queue = append(queue, q)
		}
	}
	return dist
}
