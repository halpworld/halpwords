package combat

import (
	"testing"

	"github.com/halpworld/halpwords/pkg/words"
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

func TestDamage(t *testing.T) {
	cases := []struct {
		tier   words.Tier
		speed  float64
		streak int
		want   int
		crit   bool
	}{
		{words.Perfect, 1, 0, 6, false},
		{words.Perfect, 2, 0, 18, true},   // 6 × 2 × 1.5
		{words.Correct, 1, 0, 5, false},   // 6 × 0.85 = 5.1
		{words.Perfect, 1, 5, 9, false},   // 6 × 1.5
		{words.Perfect, 1, 50, 12, false}, // combo capped at 2
		{words.Graze, 0.5, 0, 1, false},   // at least 1 when it hits at all
		{words.Miss, 2, 5, 0, false},
	}
	for _, c := range cases {
		got, crit := Damage(6, c.tier, c.speed, c.streak)
		if got != c.want || crit != c.crit {
			t.Errorf("Damage(6, %v, %v, %d) = %d, %v; want %d, %v", c.tier, c.speed, c.streak, got, crit, c.want, c.crit)
		}
	}
}

func TestBlock(t *testing.T) {
	if Block(words.Perfect) != 0 || Block(words.Correct) != 0 {
		t.Error("clean answers should dodge")
	}
	if Block(words.AccentSlip) != 0.5 || Block(words.Graze) != 0.5 {
		t.Error("slips should halve the hit")
	}
	if Block(words.Miss) != 1 {
		t.Error("a miss should take the full hit")
	}
}

func TestSpeedWindow(t *testing.T) {
	for n := 1; n <= 12; n++ {
		w := SpeedWindow(n)
		if s := Speed(n, w); s != 0.5 {
			t.Errorf("Speed(%d, window) = %v, want 0.5", n, s)
		}
		if s := Speed(n, w*0.99); s <= 0.5 {
			t.Errorf("Speed(%d, just inside window) = %v, want above 0.5", n, s)
		}
	}
}
