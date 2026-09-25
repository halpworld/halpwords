package sim

import (
	"testing"

	"github.com/halpworld/halpwords/assets"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/pkg/words"
)

// These tests are guard rails for the game's balance. If a change to the
// monsters, the hero or the formulas trips one, run tools/balance to see
// the whole picture before changing the numbers here.

func entries(t *testing.T) []words.Entry {
	lists, err := words.LoadFS(assets.Words, "words")
	if err != nil {
		t.Fatal(err)
	}
	var out []words.Entry
	for _, l := range lists {
		if l.Language == "fr" {
			out = append(out, l.Entries...)
		}
	}
	return out
}

func summary(t *testing.T, typist string, class rpg.Class, floors int) []Summary {
	var ty Typist
	for _, x := range Typists {
		if x.Name == typist {
			ty = x
		}
	}
	e := entries(t)
	var runs [][]FloorStats
	for seed := uint64(1); seed <= 60; seed++ {
		runs = append(runs, Run(ty, class, e, seed, floors))
	}
	return Summarize(runs)
}

func TestRunIsDeterministic(t *testing.T) {
	e := entries(t)
	a, b := Run(Typists[1], rpg.Rogue, e, 9, 4), Run(Typists[1], rpg.Rogue, e, 9, 4)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("floor %d: %+v then %+v", i+1, a[i], b[i])
		}
	}
}

func TestFirstFloorsAreGentle(t *testing.T) {
	// A beginner of any class gets through the first floors, where they
	// learn to play.
	for _, c := range rpg.Classes {
		for _, s := range summary(t, "beginner", c, 3) {
			if s.FellOnce > 0.08 {
				t.Errorf("%s beginner: %.0f%% fall on floor %d", c, 100*s.FellOnce, s.Depth)
			}
		}
	}
}

func TestDepthGetsHarder(t *testing.T) {
	for _, c := range rpg.Classes {
		s := summary(t, "average", c, 12)
		first, last := s[0].HPPer, s[11].HPPer
		if last < first*2 {
			t.Errorf("%s: fights cost %.0f%% of HP on floor 1 and %.0f%% on floor 12; deeper should hurt more", c, 100*first, 100*last)
		}
		if last > 0.2 {
			t.Errorf("%s: fights on floor 12 cost %.0f%% of HP, too much for an average typist", c, 100*last)
		}
		for _, f := range s[:9] {
			if f.FellOnce > 0.1 {
				t.Errorf("%s average: %.0f%% fall on floor %d", c, 100*f.FellOnce, f.Depth)
			}
		}
		if boss := s[8]; boss.BossHP < 0.15 || boss.BossHP > 0.6 {
			t.Errorf("%s: the floor 9 boss takes %.0f%% of an average hero's HP", c, 100*boss.BossHP)
		}
	}
}

func TestFightsAreShort(t *testing.T) {
	// Every fight asks for a few words, but none drags on.
	for _, ty := range []string{"beginner", "average", "strong"} {
		for _, s := range summary(t, ty, rpg.Knight, 9) {
			if s.TurnsPer < 1.5 || s.TurnsPer > 7 {
				t.Errorf("%s: %.1f attacks a fight on floor %d", ty, s.TurnsPer, s.Depth)
			}
		}
	}
}

func TestClassesAreFair(t *testing.T) {
	// No class should be much safer than another for the same player.
	hurt := map[rpg.Class]float64{}
	for _, c := range rpg.Classes {
		for _, s := range summary(t, "average", c, 9) {
			hurt[c] += s.HPPer
		}
	}
	lo, hi := 1e9, 0.0
	for _, v := range hurt {
		lo, hi = min(lo, v), max(hi, v)
	}
	if hi > lo*1.6 {
		t.Errorf("classes differ too much in how much fights hurt: %v", hurt)
	}
}
