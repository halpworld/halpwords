package dungeon

import (
	"math/rand/v2"
	"sort"
)

// Size returns the map width and height for a floor depth. Deeper floors are
// bigger.
func Size(depth int) int { return min(24+depth*2, 40) }

// Generate builds a floor from rooms joined by corridors. The same seed and
// depth always give the same floor.
func Generate(seed uint64, depth int) *Level {
	rng := rand.New(rand.NewPCG(seed, uint64(depth)*0x9e3779b97f4a7c15+1))
	n := Size(depth)
	f := &Level{
		W: n, H: n, Depth: depth, Seed: seed,
		tiles:   make([]Tile, n*n),
		Seen:    make([]bool, n*n),
		Torches: make([]bool, n*n),
		Chests:  map[Point]*Chest{},
	}
	f.placeRooms(rng)
	f.connectRooms(rng)
	f.placeDoors(rng)
	f.placeStartAndExit(rng)
	f.placeTorches(rng)
	f.placeChests(rng)
	f.placeMonsters(rng)
	return f
}

func (f *Level) carve(r Room) {
	for y := r.Y; y < r.Y+r.H; y++ {
		for x := r.X; x < r.X+r.W; x++ {
			f.Set(Point{x, y}, Floor)
		}
	}
}

func (f *Level) placeRooms(rng *rand.Rand) {
	target := 6 + f.Depth/2 + rng.IntN(3)
	for tries := 0; tries < 400 && len(f.Rooms) < target; tries++ {
		r := Room{W: 3 + rng.IntN(4), H: 3 + rng.IntN(4)}
		r.X = 1 + rng.IntN(f.W-r.W-2)
		r.Y = 1 + rng.IntN(f.H-r.H-2)
		ok := true
		for _, o := range f.Rooms {
			// Keep at least two walls between rooms so corridors and doors fit.
			if r.X-2 < o.X+o.W && o.X-2 < r.X+r.W && r.Y-2 < o.Y+o.H && o.Y-2 < r.Y+r.H {
				ok = false
				break
			}
		}
		if ok {
			f.Rooms = append(f.Rooms, r)
			f.carve(r)
		}
	}
}

// connectRooms joins the rooms with a minimum spanning tree, so every room is
// reachable, plus a few extra corridors to make loops.
func (f *Level) connectRooms(rng *rand.Rand) {
	type edge struct{ a, b, d int }
	var edges []edge
	for i := range f.Rooms {
		for j := i + 1; j < len(f.Rooms); j++ {
			edges = append(edges, edge{i, j, f.Rooms[i].Center().Manhattan(f.Rooms[j].Center())})
		}
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].d < edges[j].d })
	parent := make([]int, len(f.Rooms))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if parent[i] != i {
			parent[i] = find(parent[i])
		}
		return parent[i]
	}
	var extra []edge
	for _, e := range edges {
		ra, rb := find(e.a), find(e.b)
		if ra != rb {
			parent[ra] = rb
			f.corridor(f.Rooms[e.a].Center(), f.Rooms[e.b].Center(), rng)
		} else {
			extra = append(extra, e)
		}
	}
	loops := 1 + len(f.Rooms)/4
	for i := 0; i < len(extra) && loops > 0; i++ {
		if rng.IntN(3) == 0 {
			f.corridor(f.Rooms[extra[i].a].Center(), f.Rooms[extra[i].b].Center(), rng)
			loops--
		}
	}
}

// corridor carves an L-shaped passage from a to b.
func (f *Level) corridor(a, b Point, rng *rand.Rand) {
	horizontalFirst := rng.IntN(2) == 0
	p := a
	stepTo := func(target Point, horizontal bool) {
		for {
			if horizontal && p.X != target.X {
				p.X += sign(target.X - p.X)
			} else if !horizontal && p.Y != target.Y {
				p.Y += sign(target.Y - p.Y)
			} else {
				return
			}
			f.Set(p, Floor)
		}
	}
	stepTo(b, horizontalFirst)
	stepTo(b, !horizontalFirst)
}

func sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

// roomAt returns the index of the room containing p, or -1.
func (f *Level) roomAt(p Point) int {
	for i, r := range f.Rooms {
		if r.Contains(p) {
			return i
		}
	}
	return -1
}

// ringCell is a cell just outside a room, and the direction away from it.
type ringCell struct {
	p   Point
	out Dir
}

// placeDoors puts doors where a one-wide corridor enters a room. The start
// room (room 0) is never sealed, so the hero can always walk out of it.
func (f *Level) placeDoors(rng *rand.Rand) {
	for i, r := range f.Rooms {
		var ring []ringCell
		for x := r.X; x < r.X+r.W; x++ {
			ring = append(ring, ringCell{Point{x, r.Y - 1}, North}, ringCell{Point{x, r.Y + r.H}, South})
		}
		for y := r.Y; y < r.Y+r.H; y++ {
			ring = append(ring, ringCell{Point{r.X - 1, y}, West}, ringCell{Point{r.X + r.W, y}, East})
		}
		for _, c := range ring {
			if f.At(c.p) != Floor || f.roomAt(c.p) >= 0 {
				continue
			}
			// A proper doorway: walls on both sides, corridor outside.
			side := c.out.Right().Delta()
			if f.At(c.p.Add(side)) != Wall || f.At(Point{c.p.X - side.X, c.p.Y - side.Y}) != Wall {
				continue
			}
			if !f.At(c.p.Step(c.out)).Walkable() {
				continue
			}
			switch v := rng.IntN(10); {
			case v < 3 && i > 0:
				f.Set(c.p, Sealed)
			case v < 7:
				f.Set(c.p, Door)
			}
		}
	}
}

func (f *Level) placeStartAndExit(rng *rand.Rand) {
	start := f.Rooms[0]
	f.Start = start.Center()
	// Face the most open direction.
	best := -1
	for d := North; d <= West; d++ {
		n := 0
		for p := f.Start.Step(d); f.At(p) == Floor && n < 10; p = p.Step(d) {
			n++
		}
		if n > best {
			best, f.StartDir = n, d
		}
	}
	// The stairs go in the room furthest from the start.
	dist := f.distances(f.Start, true)
	far, farD := 1, -1
	for i, r := range f.Rooms[1:] {
		if d := dist[f.Index(r.Center())]; d > farD {
			far, farD = i+1, d
		}
	}
	exit := f.Rooms[far]
	f.Exit = Point{exit.X + rng.IntN(exit.W), exit.Y + rng.IntN(exit.H)}
	f.Set(f.Exit, Stairs)
}

// placeTorches mounts torches on some room walls.
func (f *Level) placeTorches(rng *rand.Rand) {
	for _, r := range f.Rooms {
		for i := 0; i < 1+rng.IntN(2); i++ {
			var p Point
			switch rng.IntN(4) {
			case 0:
				p = Point{r.X + rng.IntN(r.W), r.Y - 1}
			case 1:
				p = Point{r.X + rng.IntN(r.W), r.Y + r.H}
			case 2:
				p = Point{r.X - 1, r.Y + rng.IntN(r.H)}
			default:
				p = Point{r.X + r.W, r.Y + rng.IntN(r.H)}
			}
			if f.At(p) == Wall && f.In(p) {
				f.Torches[f.Index(p)] = true
			}
		}
	}
}

// quietCells returns room cells with no openings next to them, where a chest
// can stand without blocking a path.
func (f *Level) quietCells(r Room) []Point {
	var out []Point
	for y := r.Y; y < r.Y+r.H; y++ {
		for x := r.X; x < r.X+r.W; x++ {
			p := Point{x, y}
			if f.At(p) != Floor || p == f.Start {
				continue
			}
			ok, walls := true, 0
			for d := North; d <= West; d++ {
				q := p.Step(d)
				if r.Contains(q) {
					continue
				}
				if f.At(q) != Wall {
					ok = false
				}
				walls++
			}
			if ok && walls > 0 {
				out = append(out, p)
			}
		}
	}
	return out
}

func (f *Level) placeChests(rng *rand.Rand) {
	want := 2 + rng.IntN(2)
	order := rng.Perm(len(f.Rooms))
	for _, i := range order {
		if want == 0 {
			break
		}
		if i == 0 {
			continue
		}
		cells := f.quietCells(f.Rooms[i])
		if len(cells) == 0 {
			continue
		}
		p := cells[rng.IntN(len(cells))]
		c := &Chest{Gold: 5 + rng.IntN(10) + f.Depth*4}
		if rng.IntN(2) == 0 {
			c.Potions = 1
		}
		f.Chests[p] = c
		want--
	}
}

func (f *Level) placeMonsters(rng *rand.Rand) {
	want := 3 + f.Depth
	var cells []Point
	for i, r := range f.Rooms {
		if i == 0 {
			continue
		}
		for y := r.Y; y < r.Y+r.H; y++ {
			for x := r.X; x < r.X+r.W; x++ {
				p := Point{x, y}
				if f.At(p) == Floor && f.Chests[p] == nil && p.Manhattan(f.Start) > 6 {
					cells = append(cells, p)
				}
			}
		}
	}
	rng.Shuffle(len(cells), func(i, j int) { cells[i], cells[j] = cells[j], cells[i] })
	for _, p := range cells {
		if len(f.Monsters) >= want {
			break
		}
		if f.MonsterAt(p) != nil {
			continue
		}
		f.Monsters = append(f.Monsters, NewMonster(RandomKind(f.Depth, rng), f.Depth, p, rng.Uint64()))
	}
}
