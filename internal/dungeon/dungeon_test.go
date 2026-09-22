package dungeon

import (
	"math/rand/v2"
	"testing"
)

func TestGenerateIsSolvable(t *testing.T) {
	for seed := uint64(1); seed <= 300; seed++ {
		for _, depth := range []int{1, 3, 8} {
			f := Generate(seed, depth)
			if len(f.Rooms) < 2 {
				t.Fatalf("seed %d depth %d: only %d rooms", seed, depth, len(f.Rooms))
			}
			if f.At(f.Start) != Floor {
				t.Fatalf("seed %d: start is not floor", seed)
			}
			if f.At(f.Exit) != Stairs || f.Exit == f.Start {
				t.Fatalf("seed %d: bad exit %v", seed, f.Exit)
			}
			dist := f.distances(f.Start, true)
			if dist[f.Index(f.Exit)] < 0 {
				t.Fatalf("seed %d depth %d: stairs unreachable", seed, depth)
			}
			// Every open cell must be reachable, and chests must not cut the
			// map in two.
			for y := 0; y < f.H; y++ {
				for x := 0; x < f.W; x++ {
					p := Point{x, y}
					if f.At(p) != Wall && dist[f.Index(p)] < 0 {
						t.Fatalf("seed %d depth %d: %v unreachable", seed, depth, p)
					}
					if (x == 0 || y == 0 || x == f.W-1 || y == f.H-1) && f.At(p) != Wall {
						t.Fatalf("seed %d: open border cell %v", seed, p)
					}
				}
			}
			for p := range f.Chests {
				for d := North; d <= West; d++ {
					if q := p.Step(d); f.At(q) == Door || f.At(q) == Sealed {
						t.Fatalf("seed %d: chest %v next to a door", seed, p)
					}
				}
			}
			for _, m := range f.Monsters {
				if !f.At(m.At).Walkable() || f.Chests[m.At] != nil {
					t.Fatalf("seed %d: monster on %v", seed, m.At)
				}
			}
		}
	}
}

func TestChestsDoNotBlockPaths(t *testing.T) {
	for seed := uint64(1); seed <= 200; seed++ {
		f := Generate(seed, 2)
		// Treat chests as walls and check the stairs are still reachable.
		for p := range f.Chests {
			f.Set(p, Wall)
		}
		if f.distances(f.Start, true)[f.Index(f.Exit)] < 0 {
			t.Fatalf("seed %d: chests block the stairs", seed)
		}
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	a, b := Generate(42, 3), Generate(42, 3)
	for i := range a.tiles {
		if a.tiles[i] != b.tiles[i] {
			t.Fatal("same seed gave different maps")
		}
	}
	if len(a.Monsters) != len(b.Monsters) || a.Exit != b.Exit {
		t.Fatal("same seed gave different contents")
	}
	c := Generate(43, 3)
	same := true
	for i := range a.tiles {
		if i < len(c.tiles) && a.tiles[i] != c.tiles[i] {
			same = false
		}
	}
	if same {
		t.Fatal("different seeds gave the same map")
	}
}

func TestMonstersChase(t *testing.T) {
	f := Generate(7, 1)
	rng := rand.New(rand.NewPCG(1, 2))
	// Put one monster a few steps from the hero in the start room's corridor.
	f.Monsters = nil
	hero := f.Start
	var spot Point
	found := false
	dist := f.distances(hero, false)
	for i, d := range dist {
		if d == 4 {
			spot = Point{i % f.W, i / f.W}
			found = true
			break
		}
	}
	if !found {
		t.Skip("no cell at distance 4")
	}
	f.Monsters = []*Monster{NewMonster(&Kinds[0], 1, spot, 1)}
	var attacker *Monster
	for turn := 0; turn < 6 && attacker == nil; turn++ {
		attacker = f.MoveMonsters(hero, rng)
	}
	if attacker == nil {
		t.Fatalf("monster did not reach the hero; it is at %v, hero at %v", f.Monsters[0].At, hero)
	}
}

func TestDirs(t *testing.T) {
	if North.Left() != West || North.Right() != East || East.Back() != West {
		t.Fatal("turning is wrong")
	}
	if (Point{3, 3}).Step(South) != (Point{3, 4}) {
		t.Fatal("south should increase y")
	}
}
