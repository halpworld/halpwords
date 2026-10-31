package combat

import (
	"math"
	"testing"

	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/pkg/words"
)

// strike mirrors the game's call: the class boosts the damage speed, the
// critical hit is judged on the real one.
func strike(class rpg.Class, atk int, tier words.Tier, real float64, streak int) (int, bool) {
	h := rpg.NewHero(class)
	return Damage(atk, tier, h.SpeedDamage(real), real, streak)
}

// The threshold is exactly 1.5 real speed, for every class.
func TestCritThresholdIsExactlyOnePointFive(t *testing.T) {
	for _, c := range rpg.Classes {
		for _, tc := range []struct {
			real float64
			want bool
		}{
			{1.4999999, false}, {math.Nextafter(1.5, 0), false}, {1.5, true},
			{math.Nextafter(1.5, 2), true}, {2, true}, {1, false}, {0.5, false}, {1.34, false},
		} {
			if _, crit := strike(c, 10, words.Perfect, tc.real, 0); crit != tc.want {
				t.Errorf("class %v real %v: crit %v, want %v", c, tc.real, crit, tc.want)
			}
		}
	}
}

// A Scribe and a Knight at the same real speed crit together; the Scribe
// does more damage (the boost) only above speed 1, and never crits sooner.
func TestScribeVersusKnightAtTheSameRealSpeed(t *testing.T) {
	for real := 0.5; real <= 2.0001; real += 0.01 {
		kd, kc := strike(rpg.Knight, 20, words.Perfect, real, 0)
		sd, sc := strike(rpg.Scribe, 20, words.Perfect, real, 0)
		if kc != sc {
			t.Fatalf("real %.2f: knight crit %v, scribe crit %v", real, kc, sc)
		}
		if real <= 1 && kd != sd {
			t.Errorf("real %.2f: damage %d vs %d below normal speed", real, kd, sd)
		}
		if sd < kd || (real > 1.2 && sd <= kd) { // whole numbers: tiny boosts round away
			t.Errorf("real %.2f: scribe %d vs knight %d", real, sd, kd)
		}
	}
	// At exactly the threshold the Scribe's damage is the boosted one × 1.5.
	d, crit := strike(rpg.Scribe, 20, words.Perfect, 1.5, 0)
	want := int(math.Round(20 * (1 + 0.5*1.5) * 1.5))
	if !crit || d != want {
		t.Errorf("scribe at 1.5: %d crit %v, want %d", d, crit, want)
	}
}

// Only a Perfect answer crits, whatever the speed; the streak raises
// damage but never decides the crit, and a miss does nothing.
func TestCritTierAndStreak(t *testing.T) {
	for _, tier := range []words.Tier{words.Miss, words.Graze, words.AccentSlip, words.Correct} {
		if _, crit := strike(rpg.Scribe, 20, tier, 2, 10); crit {
			t.Errorf("tier %v crit", tier)
		}
	}
	base, _ := strike(rpg.Knight, 20, words.Perfect, 1.49, 0)
	for _, streak := range []int{0, 1, 5, 10, 50, 1000} {
		_, crit := strike(rpg.Knight, 20, words.Perfect, 1.49, streak)
		if crit {
			t.Errorf("streak %d made a crit at 1.49", streak)
		}
		_, crit = strike(rpg.Knight, 20, words.Perfect, 1.5, streak)
		if !crit {
			t.Errorf("streak %d lost the crit at 1.5", streak)
		}
		d, _ := strike(rpg.Knight, 20, words.Perfect, 1.49, streak)
		if d < base {
			t.Errorf("streak %d lowered the damage: %d < %d", streak, d, base)
		}
	}
	if d, crit := strike(rpg.Knight, 20, words.Miss, 2, 5); d != 0 || crit {
		t.Errorf("miss: %d %v", d, crit)
	}
	// Combo stays capped at 2× with a crit on top.
	d, crit := strike(rpg.Knight, 10, words.Perfect, 2, 1000)
	if !crit || d != 10*2*2*3/2 {
		t.Errorf("capped combo crit: %d %v", d, crit)
	}
}

// A real speed above 1.5 with the boost not applied (Knight) and a boosted
// speed that is high though the real one is low: only real counts.
func TestCritIgnoresTheBoostedSpeed(t *testing.T) {
	if _, crit := Damage(10, words.Perfect, 1.9, 1.2, 0); crit {
		t.Error("boosted 1.9 / real 1.2 crit")
	}
	if _, crit := Damage(10, words.Perfect, 1.0, 1.6, 0); !crit {
		t.Error("boosted 1.0 / real 1.6 no crit")
	}
	if d, _ := Damage(1, words.Graze, 0.5, 0.5, 0); d != 1 {
		t.Errorf("minimum damage %d", d)
	}
}
