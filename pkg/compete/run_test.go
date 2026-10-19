package compete

import (
	"errors"
	"testing"
	"time"

	"github.com/halpworld/halpwords/pkg/words"
)

func TestListHash(t *testing.T) {
	a := []words.Entry{{Prompt: "cat", Answers: []string{"chat"}}, {Prompt: "dog", Answers: []string{"chien"}}}
	b := []words.Entry{a[1], a[0]}
	if ListHash(a) != ListHash(b) {
		t.Error("the order of the words changes the hash")
	}
	if h := ListHash(a); len(h) != 16 || h == ListHash(a[:1]) {
		t.Errorf("hash %q", h)
	}
}

func TestRunCheck(t *testing.T) {
	tally := Tally{Damage: 900, Perfect: 40, BestCombo: 12, Bosses: 1, Chests: 6, Misses: 5}
	good := Run{Share: Share{Lang: "fr", Seed: 42, Floor: 5, Score: tally.Score(5)}, Tally: tally, Secs: 600}
	if err := good.Check(); err != nil {
		t.Fatalf("a real run: %v", err)
	}
	change := func(f func(r *Run)) Run {
		r := good
		f(&r)
		return r
	}
	for name, r := range map[string]Run{
		"score raised":   change(func(r *Run) { r.Share.Score += 1000 }),
		"tally raised":   change(func(r *Run) { r.Tally.Bosses += 1 }),
		"no floor":       change(func(r *Run) { r.Share.Floor = 0 }),
		"too fast":       change(func(r *Run) { r.Share.Floor, r.Secs = 50, 10; r.Share.Score = r.Tally.Score(50) }),
		"too many words": change(func(r *Run) { r.Tally.Perfect = 5000; r.Share.Score = r.Tally.Score(5) }),
		"too many bosses": change(func(r *Run) {
			r.Tally.Bosses = 6
			r.Share.Score = r.Tally.Score(5)
		}),
		"too many chests": change(func(r *Run) { r.Tally.Chests = 40; r.Share.Score = r.Tally.Score(5) }),
		"too much damage": change(func(r *Run) { r.Tally.Damage = 1_000_000; r.Share.Score = r.Tally.Score(5) }),
		"negative":        change(func(r *Run) { r.Tally.Misses = -3; r.Share.Score = r.Tally.Score(5) }),
	} {
		if err := r.Check(); !errors.Is(err, ErrImplausible) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// A run counts on the rankings only once a floor is cleared (#93): one
// lost or given up on floor 1 doesn't, whatever its score.
func TestRanked(t *testing.T) {
	for floor, want := range map[int]bool{0: false, 1: false, 2: true, 3: true, MaxFloor: true} {
		tally := Tally{Damage: 300, Perfect: 5}
		r := Run{Share: Share{Lang: "fr", Seed: 42, Floor: floor, Score: tally.Score(floor)}, Tally: tally, Secs: 600}
		if r.Ranked() != want || r.Share.Ranked() != want {
			t.Errorf("floor %d: ranked %v, want %v", floor, r.Ranked(), want)
		}
	}
	// Floor 1 with no play is plausible (Check passes) but not ranked.
	idle := Run{Share: Share{Lang: "fr", Seed: 42, Floor: 1, Score: Tally{}.Score(1)}}
	if err := idle.Check(); err != nil || idle.Ranked() {
		t.Fatalf("an idle floor-1 run: check %v, ranked %v", err, idle.Ranked())
	}
	daily := Share{Lang: "fr", Daily: true, Month: 9, Day: 22, Floor: 1, Score: 1000}
	if daily.Ranked() {
		t.Fatal("a Daily Dungeon on floor 1 is ranked")
	}
}

func TestDailySendWindow(t *testing.T) {
	if DailySendWindow != 15*time.Minute {
		t.Fatalf("DailySendWindow = %v, the owner chose 15 minutes", DailySendWindow)
	}
	end := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		after time.Duration
		late  bool
	}{{0, false}, {14 * time.Minute, false}, {DailySendWindow, false}, {DailySendWindow + time.Second, true}, {time.Hour, true}} {
		if got := DailyTooLate(end, end.Add(c.after)); got != c.late {
			t.Errorf("DailyTooLate after %v = %v, want %v", c.after, got, c.late)
		}
	}
}
