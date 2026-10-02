package rpg

import "testing"

// GoldFind is integer maths: gold*(100+5*luck [+50 rogue chest]) rounded.
func TestQAGoldFindIntegerTable(t *testing.T) {
	for _, c := range Classes {
		for luck := 0; luck <= 40; luck++ {
			h := NewHero(c)
			h.Base.Luck = luck - h.Stats().Luck + h.Base.Luck
			l := h.Stats().Luck
			for _, chest := range []bool{false, true} {
				for _, gold := range []int{0, 1, 2, 3, 5, 7, 10, 13, 99, 250, 1001} {
					pct := 100 + 5*l
					if chest && c == Rogue {
						pct += 50
					}
					want := (gold*pct + 50) / 100
					if got := h.GoldFind(gold, chest); got != want {
						t.Errorf("%v luck %d chest %v gold %d: %d, want %d", c, l, chest, gold, got, want)
					}
				}
			}
		}
	}
	h := NewHero(Knight)
	h.Base.Luck = 1
	if got := h.GoldFind(10, false); got != 11 { // 10*105=1050 -> 11 (rounds half up)
		t.Errorf("GoldFind(10) at luck 1 = %d, want 11", got)
	}
}
