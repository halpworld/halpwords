package raid

import (
	"testing"
	"time"

	"github.com/halpworld/halpwords/pkg/words"
)

func TestDamageFollowsTheTiers(t *testing.T) {
	prev := -1
	for _, tier := range []words.Tier{words.Miss, words.Graze, words.AccentSlip, words.Correct, words.Perfect} {
		d := Damage(tier, 0)
		if d <= prev && tier != words.Miss {
			t.Errorf("tier %v deals %d, no more than the tier below (%d)", tier, d, prev)
		}
		prev = d
	}
	if Damage(words.Miss, 9) != 0 {
		t.Error("a miss hurts the boss")
	}
	if Damage(words.Perfect, 0) != Hit || Damage(words.Perfect, 100) != 2*Hit {
		t.Errorf("perfect: %d, on a long streak %d", Damage(words.Perfect, 0), Damage(words.Perfect, 100))
	}
}

func TestStreak(t *testing.T) {
	s := Streak(Streak(0, words.Perfect), words.Correct)
	if s != 2 || Streak(s, words.AccentSlip) != 2 || Streak(s, words.Graze) != 0 || Streak(s, words.Miss) != 0 {
		t.Errorf("streaks: %d", s)
	}
}

// TestAClassCanWin checks the boss's health against steady answering: a
// class answering one word in five seconds, most of them right, wins well
// inside the time limit, and one answering nothing doesn't.
func TestAClassCanWin(t *testing.T) {
	for _, n := range []int{MinRaiders, 12, MaxRaiders} {
		hp, streak := BossHP(n), 0
		var took time.Duration
		for i := 0; hp > 0; i++ {
			tier := words.Correct
			if i%4 == 3 {
				tier = words.Miss
			}
			streak = Streak(streak, tier)
			hp -= n * Damage(tier, streak)
			took += 5 * time.Second
		}
		if took > TimeLimit*3/4 || took < TimeLimit/4 {
			t.Errorf("%d raiders took %v", n, took)
		}
	}
}

func TestAttacksQuicken(t *testing.T) {
	if !(AttackEvery(100, 100) > AttackEvery(60, 100) && AttackEvery(60, 100) > AttackEvery(20, 100)) {
		t.Error("attacks don't quicken as the boss weakens")
	}
	if DodgeTime(5) <= DodgeTime(1) || DodgeTime(1) < 3*time.Second {
		t.Errorf("dodge times: %v, %v", DodgeTime(1), DodgeTime(5))
	}
	if !Dodged(words.AccentSlip) || Dodged(words.Graze) {
		t.Error("dodges")
	}
}

// TestGradeIsTheGames checks Grade against words.Grade with the
// language's own defaults, the grading a practising game uses.
func TestGradeIsTheGames(t *testing.T) {
	e := words.Entry{Prompt: "the dog", Answers: []string{"le chien"}}
	fr, _ := words.Lookup("fr")
	for _, typed := range []string{"le chien", "chien", "le chein", "Le Chien", "", "la chienne", "le chièn"} {
		for _, bs := range []bool{false, true} {
			got, want := Grade(typed, e, "fr", bs), words.Grade(typed, e, fr, fr.Defaults, bs)
			if got != want {
				t.Errorf("%q (backspace %v): %+v, want %+v", typed, bs, got, want)
			}
		}
	}
	if Grade("x", e, "zz", false).Tier != words.Miss || Grade("le chien", e, "zz", false).Tier != words.Perfect {
		t.Error("an unknown language")
	}
}

func TestBoss(t *testing.T) {
	seen := map[string]bool{}
	for s := range uint64(20) {
		b := BossFor(s)
		seen[b.Name] = true
		m := b.Sprite(int(s))
		if m == nil || m.W == 0 || m.H <= m.W {
			t.Fatalf("%s: a sprite without its crown", b.Name)
		}
	}
	if len(seen) < 5 {
		t.Errorf("only %d bosses", len(seen))
	}
}
