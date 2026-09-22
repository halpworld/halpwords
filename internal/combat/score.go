// Package combat holds the battle formulas.
package combat

import "github.com/halpworld/halpwords/internal/words"

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
