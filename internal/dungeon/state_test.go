package dungeon

import (
	"encoding/json"
	"math/rand/v2"
	"reflect"
	"testing"
)

// A floor saved mid-play and restored onto a freshly generated copy of it
// must come back exactly as it was.
func TestStateRoundTrip(t *testing.T) {
	for _, depth := range []int{1, 3, 6} {
		f := Generate(42, depth)
		rng := rand.New(rand.NewPCG(1, 2))
		for i := 0; i < 20; i++ {
			f.MoveMonsters(f.Start, rng)
		}
		for i := 0; i < len(f.Seen); i += 3 {
			f.Seen[i] = true
		}
		for p, c := range f.Chests {
			if c.Mimic {
				f.WakeMimic(p, 7)
			} else {
				c.Open = true
			}
			break
		}
		for y := 0; y < f.H; y++ {
			for x := 0; x < f.W; x++ {
				if p := (Point{x, y}); f.At(p) == Door {
					f.Set(p, OpenDoor)
				}
			}
		}
		f.Monsters[0].HP = 1
		f.Monsters[0].Stun = 3

		data, err := json.Marshal(f.State())
		if err != nil {
			t.Fatal(err)
		}
		var s State
		if err := json.Unmarshal(data, &s); err != nil {
			t.Fatal(err)
		}
		g := Generate(42, depth)
		if err := g.Restore(s); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(f, g) {
			t.Fatalf("depth %d: restored floor differs", depth)
		}
	}
}

func TestRestoreRejectsOtherFloor(t *testing.T) {
	s := Generate(1, 1).State()
	if err := Generate(1, 9).Restore(s); err == nil {
		t.Fatal("restored a state onto a floor of another size")
	}
	s = Generate(1, 1).State()
	s.Monsters = append(s.Monsters, MonsterState{Kind: "Dragon"})
	if err := Generate(1, 1).Restore(s); err == nil {
		t.Fatal("restored an unknown monster")
	}
}

func TestKindNamed(t *testing.T) {
	for i := range Kinds {
		if KindNamed(Kinds[i].Name) != &Kinds[i] {
			t.Fatalf("%s not found", Kinds[i].Name)
		}
	}
	if KindNamed("Mimic") != &MimicKind {
		t.Fatal("Mimic not found")
	}
}
