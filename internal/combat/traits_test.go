package combat

import (
	"testing"

	"github.com/halpworld/halpwords/pkg/words"
)

func TestArmorBlocks(t *testing.T) {
	for tier, want := range map[words.Tier]bool{
		words.Perfect:    false,
		words.Correct:    false,
		words.AccentSlip: true,
		words.Graze:      true,
		words.Miss:       true,
	} {
		if got := ArmorBlocks(tier); got != want {
			t.Errorf("ArmorBlocks(%v) = %v, want %v", tier, got, want)
		}
	}
}

func TestVisibility(t *testing.T) {
	for _, c := range []struct{ t, want float64 }{
		{0, 1}, {FadeStart, 1}, {(FadeStart + FadeEnd) / 2, 0.5}, {FadeEnd, 0}, {10, 0},
	} {
		if got := Visibility(c.t); got != c.want {
			t.Errorf("Visibility(%v) = %v, want %v", c.t, got, c.want)
		}
	}
}

func TestMirror(t *testing.T) {
	for in, want := range map[string]string{
		"dog":       "god",
		"ice cream": "maerc eci",
		"":          "",
		"été":       "été",
		"château":   "uaetâhc",
	} {
		if got := Mirror(in); got != want {
			t.Errorf("Mirror(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSwiftShortensDodges(t *testing.T) {
	if SwiftTime <= 0 || SwiftTime >= 1 {
		t.Fatalf("SwiftTime = %v, want between 0 and 1", SwiftTime)
	}
	// Even a swift monster leaves time to type a long word.
	if n := 12; DefendTime(n)*SwiftTime < TargetTime(n) {
		t.Errorf("swift dodge time %.1fs is under the target time %.1fs", DefendTime(n)*SwiftTime, TargetTime(n))
	}
}
