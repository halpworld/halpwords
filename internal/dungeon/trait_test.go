package dungeon

import (
	"math/rand/v2"
	"testing"
)

func TestFirstFloorHasNoTraits(t *testing.T) {
	for _, k := range Kinds {
		if k.MinDep <= 1 && k.Traits != 0 {
			t.Errorf("%s appears on floor 1 with traits %v", k.Name, k.Traits)
		}
	}
	for seed := uint64(0); seed < 20; seed++ {
		for depth := 1; depth <= 3; depth++ {
			for _, m := range Generate(seed, depth).Monsters {
				if m.Extra != 0 {
					t.Errorf("seed %d floor %d: %s has an extra trait", seed, depth, m.Name())
				}
			}
		}
	}
}

func TestEveryTraitAppears(t *testing.T) {
	var seen Trait
	for _, k := range Kinds {
		seen |= k.Traits
	}
	for _, tr := range Traits {
		if seen&tr == 0 {
			t.Errorf("no monster kind is %v", tr)
		}
		if tr.Hint() == "" || tr.String() == "" {
			t.Errorf("trait %d has no name or hint", tr)
		}
	}
}

func TestExtraTraits(t *testing.T) {
	if c := ExtraTraitChance(20); c > 0.4 {
		t.Errorf("chance on floor 20 = %v, want at most 0.4", c)
	}
	rng := rand.New(rand.NewPCG(1, 2))
	extras := 0
	for i := 0; i < 500; i++ {
		m := NewMonster(&Kinds[0], 10, Point{}, 1)
		m.maybeAddTrait(10, rng)
		if m.Extra == 0 {
			continue
		}
		extras++
		if !m.Has(m.Extra) {
			t.Fatalf("%s lacks its extra trait", m.Name())
		}
		if want := m.Extra.String() + " " + Kinds[0].Name; m.Name() != want {
			t.Fatalf("name %q, want %q", m.Name(), want)
		}
	}
	if extras < 150 || extras > 250 {
		t.Errorf("%d of 500 monsters on floor 10 got an extra trait, want about 200", extras)
	}
	// An extra trait is never one the kind already has.
	for i := 0; i < 200; i++ {
		m := NewMonster(&Kinds[4], 10, Point{}, 1) // Wisp: Ghostly
		m.maybeAddTrait(10, rng)
		if m.Extra == Ghostly {
			t.Fatal("a Wisp got Ghostly as an extra trait")
		}
	}
}

func TestTraitString(t *testing.T) {
	if s := (Ghostly | Swift).String(); s != "Ghostly, Swift" {
		t.Errorf("got %q", s)
	}
}
