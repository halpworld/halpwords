package dungeon

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/halpworld/halpwords/pkg/maps"
	"github.com/halpworld/halpwords/pkg/puzzle"
)

// Lock is the puzzle a hand-made map sets on a sealed door or chest.
type Lock struct {
	// Kind is the kind of puzzle, when Set; otherwise it is random.
	Kind puzzle.Kind
	Set  bool
	// Word is the word the puzzle is about (see maps.FindWord), or "" for
	// one the deck deals.
	Word string
}

// FromMap builds a floor from a hand-made map instead of the generator.
// The map must pass maps.Check. Everything the map leaves open is rolled
// from seed as on a generated floor: what chests hold, what a merchant
// sells, the monsters' looks. The same map and seed always give the same
// floor.
func FromMap(m *maps.Map, seed uint64) (*Level, error) {
	if ps := m.Check(nil); len(ps) > 0 {
		return nil, errors.New(ps[0].String())
	}
	depth := m.Level()
	rng := rand.New(rand.NewPCG(seed, uint64(depth)*0x9e3779b97f4a7c15+1))
	w, h := m.Size()
	f := &Level{
		W: w, H: h, Depth: depth, Seed: seed,
		tiles:    make([]Tile, w*h),
		Seen:     make([]bool, w*h),
		Torches:  make([]bool, w*h),
		Chests:   map[Point]*Chest{},
		Features: map[Point]*Feature{},
		Locks:    map[Point]Lock{},
		Notes:    map[Point]string{},
	}
	mimic := map[Point]bool{}
	for _, l := range m.Locks {
		p := Point{l.X, l.Y}
		mimic[p] = l.Mimic
		k, ok := maps.PuzzleNames[l.Puzzle]
		if ok || l.Word != "" {
			f.Locks[p] = Lock{Kind: k, Set: ok, Word: l.Word}
		}
	}
	for y := range h {
		for x := range w {
			p := Point{x, y}
			t := Floor
			switch m.At(x, y) {
			case maps.Wall:
				t = Wall
			case maps.Torch:
				t = Wall
				f.Torches[f.Index(p)] = true
			case maps.Start:
				f.Start = p
			case maps.Door:
				t = Door
			case maps.Sealed:
				t = Sealed
			case maps.Stairs:
				t = Stairs
				f.Exit = p
			case maps.Chest:
				c := f.newChest(rng)
				c.Mimic = mimic[p]
				f.Chests[p] = c
			case maps.Shrine:
				f.Features[p] = &Feature{Kind: Shrine}
			case maps.Campfire:
				f.Features[p] = &Feature{Kind: Campfire}
			case maps.Merchant:
				f.Features[p] = &Feature{Kind: Merchant, Stock: f.stock(rng)}
			}
			f.Set(p, t)
		}
	}
	f.StartDir = f.openest(f.Start)
	for d := North; d <= West; d++ {
		if m.Facing == d.String() {
			f.StartDir = d
		}
	}
	for _, mo := range m.Monsters {
		k := KindNamed(mo.Kind)
		level := mo.Level
		if level == 0 {
			level = depth
		}
		mon := NewMonster(k, level, Point{mo.X, mo.Y}, rng.Uint64())
		if t := TraitNamed(mo.Trait); t != 0 && !mon.Has(t) {
			mon.Extra = t
			mon.Traits |= t
		}
		if k.Boss() {
			if d, ok := dirBetween(mon.At, f.Exit); ok {
				mon.Facing = d.Back() // guarding: back to the stairs
			}
		}
		f.Monsters = append(f.Monsters, mon)
	}
	for _, n := range m.Notes {
		f.Notes[Point{n.X, n.Y}] = n.Text
	}
	return f, nil
}

// TraitNamed returns the trait called name, such as "swift", ignoring
// case, or 0.
func TraitNamed(name string) Trait {
	for _, t := range Traits {
		if strings.EqualFold(t.String(), name) {
			return t
		}
	}
	return 0
}

// ToMap writes the floor as it is now as a hand-made map in language lang,
// a starting point for an editor and a check that generated floors follow
// the map rules. Open doors become floor; chests keep only whether they
// are Mimics.
func ToMap(f *Level, lang string) *maps.Map {
	m := &maps.Map{
		Format: maps.MapFormat, Version: maps.Version, Language: lang,
		Title: fmt.Sprintf("Floor %d", f.Depth), Depth: f.Depth, Facing: f.StartDir.String(),
	}
	for y := range f.H {
		row := make([]byte, f.W)
		for x := range f.W {
			p := Point{x, y}
			c := maps.Floor
			switch f.At(p) {
			case Wall:
				c = maps.Wall
				if f.Torches[f.Index(p)] {
					c = maps.Torch
				}
			case Door:
				c = maps.Door
			case Sealed:
				c = maps.Sealed
			case Stairs:
				c = maps.Stairs
			}
			if ch := f.Chests[p]; ch != nil {
				c = maps.Chest
			}
			if ft := f.Features[p]; ft != nil {
				c = [...]maps.Cell{maps.Shrine, maps.Campfire, maps.Merchant}[ft.Kind%3]
			}
			if p == f.Start {
				c = maps.Start
			}
			row[x] = c
		}
		m.Rows = append(m.Rows, string(row))
	}
	for y := range f.H {
		for x := range f.W {
			p := Point{x, y}
			l, set := f.Locks[p]
			ch := f.Chests[p]
			if !set && (ch == nil || !ch.Mimic) {
				continue
			}
			ml := maps.Lock{X: x, Y: y, Word: l.Word, Mimic: ch != nil && ch.Mimic}
			if l.Set {
				for name, k := range maps.PuzzleNames {
					if k == l.Kind {
						ml.Puzzle = name
					}
				}
			}
			m.Locks = append(m.Locks, ml)
		}
	}
	for _, mo := range f.Monsters {
		if mo.HP <= 0 || mo.Kind.Family == Mimic {
			continue
		}
		mm := maps.Monster{X: mo.At.X, Y: mo.At.Y, Kind: mo.Kind.Name, Level: f.Depth}
		if mo.Extra != 0 {
			mm.Trait = strings.ToLower(mo.Extra.String())
		}
		m.Monsters = append(m.Monsters, mm)
	}
	for y := range f.H {
		for x := range f.W {
			if n, ok := f.Notes[Point{x, y}]; ok {
				m.Notes = append(m.Notes, maps.Note{X: x, Y: y, Text: n})
			}
		}
	}
	return m
}
