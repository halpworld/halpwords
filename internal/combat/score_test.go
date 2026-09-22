package combat

import (
	"testing"

	"github.com/halpworld/halpwords/internal/words"
)

func TestSpeed(t *testing.T) {
	if s := Speed(5, TargetTime(5)); s != 1 {
		t.Errorf("on-target speed = %v, want 1", s)
	}
	if s := Speed(5, 100); s != 0.5 {
		t.Errorf("slow speed = %v, want 0.5", s)
	}
	if s := Speed(5, 0.01); s != 2 {
		t.Errorf("fast speed = %v, want 2", s)
	}
}

func TestAccuracyOrder(t *testing.T) {
	prev := -1.0
	for tier := words.Miss; tier <= words.Perfect; tier++ {
		a := Accuracy(tier)
		if a <= prev {
			t.Errorf("Accuracy(%v) = %v not above %v", tier, a, prev)
		}
		prev = a
	}
}
