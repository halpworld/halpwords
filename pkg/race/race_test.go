package race

import (
	"errors"
	"testing"
	"time"
)

// walk is an honest racer: it walks steps cells a floor at full speed,
// beats a monster every few steps, and reports every half second.
func walk(t *testing.T, j *Judge, delay func(i int) time.Duration) time.Duration {
	t.Helper()
	var at time.Duration
	r := Report{Floor: 1, X: 5, Y: 5}
	i := 0
	send := func() {
		t.Helper()
		i++
		if err := j.Check(r, at+delay(i)); err != nil {
			t.Fatalf("report %d (%v) at %v: %v", i, r, at, err)
		}
	}
	send()
	for r.Floor < Goal {
		for s := 0; s < 20; s++ {
			at += StepTime
			if s%2 == 0 {
				r.X++
			} else {
				r.Y++
			}
			if s%7 == 6 {
				at += MonsterTime + time.Second
				r.Monsters++
			}
			if s%3 == 2 {
				send()
			}
		}
		at += StepTime
		r.Floor++
		r.X, r.Y = 3, 3
		send()
	}
	return at
}

func TestHonestRacerPasses(t *testing.T) {
	var j Judge
	walk(t, &j, func(int) time.Duration { return 0 })
	if !j.Last().Finished() {
		t.Fatalf("last = %v, want finished", j.Last())
	}
}

func TestLateBunchedReportsPass(t *testing.T) {
	// Every fifth report is two seconds late, so it arrives with the
	// next ones.
	var j Judge
	walk(t, &j, func(i int) time.Duration {
		if i%5 == 0 {
			return 2 * time.Second
		}
		return 0
	})
}

func TestImplausibleReports(t *testing.T) {
	type step struct {
		r  Report
		at time.Duration
	}
	for _, tc := range []struct {
		name  string
		steps []step
		want  string
	}{
		{"floor skipped", []step{{Report{Floor: 1}, time.Second}, {Report{Floor: 3}, time.Minute}}, ReasonFloor},
		{"floor back", []step{{Report{Floor: 1}, time.Second}, {Report{Floor: 2}, time.Minute}, {Report{Floor: 1}, 2 * time.Minute}}, ReasonFloor},
		{"floor zero", []step{{Report{Floor: 0}, time.Second}}, ReasonFloor},
		{"past the goal", []step{{Report{Floor: 1}, time.Second}, {Report{Floor: 2}, time.Minute}, {Report{Floor: 3}, 2 * time.Minute}, {Report{Floor: 4}, 3 * time.Minute}}, ReasonOver},
		{"straight to the stairs", []step{{Report{Floor: 2}, 200 * time.Millisecond}}, ReasonTooSoon},
		{"a crowd of monsters", []step{{Report{Floor: 1, Monsters: 30}, 5 * time.Second}}, ReasonMonsters},
		{"more than the floor holds", []step{{Report{Floor: 1, Monsters: MaxMonsters(1) + 1}, 5 * time.Minute}}, ReasonMonsters},
		{"monsters come back", []step{{Report{Floor: 1, Monsters: 2}, time.Minute}, {Report{Floor: 1, Monsters: 1}, 2 * time.Minute}}, ReasonMonsters},
		{"outside the floor", []step{{Report{Floor: 1, X: MapSize(1), Y: 2}, time.Second}}, ReasonPosition},
		{"negative cell", []step{{Report{Floor: 1, X: -1}, time.Second}}, ReasonPosition},
		{"teleporting", []step{
			{Report{Floor: 1, X: 1, Y: 1}, 10 * time.Second},
			{Report{Floor: 1, X: 25, Y: 25}, 10*time.Second + 100*time.Millisecond},
			{Report{Floor: 1, X: 1, Y: 1}, 10*time.Second + 200*time.Millisecond},
			{Report{Floor: 1, X: 25, Y: 25}, 10*time.Second + 300*time.Millisecond},
		}, ReasonSpeed},
		{"floors too fast after waiting", []step{
			{Report{Floor: 1}, 5 * time.Minute},
			{Report{Floor: 2}, 5*time.Minute + 10*time.Millisecond},
			{Report{Floor: 2, X: 20, Y: 20}, 5*time.Minute + 20*time.Millisecond},
			{Report{Floor: 2, X: 0, Y: 0}, 5*time.Minute + 30*time.Millisecond},
			{Report{Floor: 2, X: 20, Y: 20}, 5*time.Minute + 35*time.Millisecond},
			{Report{Floor: 3}, 5*time.Minute + 40*time.Millisecond},
		}, ReasonSpeed},
		{"moving after falling", []step{{Report{Floor: 1, Fell: true}, time.Minute}, {Report{Floor: 1, X: 2, Fell: true}, 2 * time.Minute}}, ReasonOver},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var j Judge
			var err error
			for _, s := range tc.steps {
				if err = j.Check(s.r, s.at); err != nil {
					break
				}
			}
			var bad *Implausible
			if !errors.As(err, &bad) || bad.Reason != tc.want {
				t.Fatalf("err = %v, want %s", err, tc.want)
			}
		})
	}
}

func TestSameReportAfterTheEndIsFine(t *testing.T) {
	var j Judge
	at := walk(t, &j, func(int) time.Duration { return 0 })
	if err := j.Check(j.Last(), at+time.Second); err != nil {
		t.Fatalf("the finishing report again: %v", err)
	}
}

func TestReporter(t *testing.T) {
	var p Reporter
	t0 := time.Unix(1000, 0)
	r := Report{Floor: 1, X: 1, Y: 1}
	if !p.Due(r, t0) {
		t.Fatal("the first report isn't due")
	}
	if p.Due(r, t0.Add(time.Second)) {
		t.Fatal("an unchanged report is due")
	}
	r.X++
	if p.Due(r, t0.Add(100*time.Millisecond)) {
		t.Fatal("a move is due before ReportEvery")
	}
	if !p.Due(r, t0.Add(ReportEvery)) {
		t.Fatal("a move isn't due after ReportEvery")
	}
	r.Floor++
	if !p.Due(r, t0.Add(ReportEvery+time.Millisecond)) {
		t.Fatal("a new floor isn't due at once")
	}
	r.Fell = true
	if !p.Due(r, t0.Add(ReportEvery+2*time.Millisecond)) {
		t.Fatal("a fall isn't due at once")
	}
}
