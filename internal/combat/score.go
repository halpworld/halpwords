// Package combat holds the battle formulas.
package combat

import "github.com/halpworld/halpwords/pkg/words"

// TargetTime is the time in seconds a fluent typist needs for an answer of n
// characters. Answering faster than this gives a speed bonus.
func TargetTime(n int) float64 {
	return 0.8 + 0.28*float64(n)
}

// Speed returns the speed multiplier for an answer typed in taken seconds.
func Speed(n int, taken float64) float64 {
	if taken <= 0 {
		return 2
	}
	return clamp(TargetTime(n)/taken, 0.5, 2)
}

// SpeedWindow is how many seconds an answer of n characters can take before
// its speed multiplier bottoms out.
func SpeedWindow(n int) float64 { return TargetTime(n) / 0.5 }

// Accuracy returns the damage multiplier for a grading tier.
func Accuracy(t words.Tier) float64 {
	switch t {
	case words.Perfect:
		return 1
	case words.Correct:
		return 0.85
	case words.AccentSlip:
		return 0.6
	case words.Graze:
		return 0.3
	default:
		return 0
	}
}

func clamp(v, lo, hi float64) float64 {
	return max(lo, min(hi, v))
}

// MaxCombo is the largest combo multiplier a streak can build.
const MaxCombo = 2.0

// Combo returns the damage multiplier for a streak of good answers in a row.
func Combo(streak int) float64 {
	return min(1+0.1*float64(streak), MaxCombo)
}

// Damage returns the damage of an attack by a hero with attack power atk,
// and whether it was a critical hit. A perfect answer typed very fast
// (speed 1.5 or more) is critical and deals half as much again.
func Damage(atk int, tier words.Tier, speed float64, streak int) (int, bool) {
	d := float64(atk) * Accuracy(tier) * speed * Combo(streak)
	crit := tier == words.Perfect && speed >= 1.5
	if crit {
		d *= 1.5
	}
	if d > 0 && d < 1 {
		d = 1
	}
	return int(d + 0.5), crit
}

// DefendTime is how long the hero has to type a dodge for an answer of n
// characters.
func DefendTime(n int) float64 { return TargetTime(n)*2 + 1.5 }

// Block returns the share of a monster's hit the hero takes after typing a
// dodge graded tier: none for a clean answer, half for a slip or graze.
func Block(tier words.Tier) float64 {
	switch {
	case tier >= words.Correct:
		return 0
	case tier >= words.Graze:
		return 0.5
	default:
		return 1
	}
}

// WordTarget is the Difficulty of the words monsters on floor depth ask
// for: short, plain words at first, longer ones deeper down and from
// bosses. It starts at 6 rather than lower, so that on the first floors a
// quick typist still needs a few words a fight when the lists hold very
// short words (numbers, colours).
func WordTarget(depth int, boss bool) float64 {
	t := max(6, 4.5+0.6*float64(depth-1))
	if boss {
		t += 2
	}
	return min(t, 14)
}

// BossPhase is how angry a boss with hp of maxHP left is: 0 at first,
// 1 below two thirds of its HP and 2 below one third.
func BossPhase(hp, maxHP int) int {
	switch {
	case hp*3 <= maxHP:
		return 2
	case hp*3 <= maxHP*2:
		return 1
	}
	return 0
}
