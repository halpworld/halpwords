package scene

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/pkg/race"
)

const raceList = `title: Race words
language: fr
the dog = le chien
the cat = le chat
the house = la maison
the tree = l'arbre
to eat = manger
to sleep = dormir
the book = le livre
the sea = la mer
`

func testRace() *link.Race {
	return &link.Race{Number: 1, Seed: 424242, Goal: race.Goal, List: raceList,
		StartsAt: time.Now(), EndsAt: time.Now().Add(race.TimeLimit)}
}

// The server judges racers by how fast the game lets them go: the crawl
// must not be quicker than pkg/race says.
func TestRaceTimingsFitTheGame(t *testing.T) {
	const tps = 60
	if d := time.Duration(stepTicks) * time.Second / tps; d < race.StepTime {
		t.Fatalf("a step takes %v, race.StepTime is %v", d, race.StepTime)
	}
	if d := time.Duration(dyingTicks) * time.Second / tps; d < race.MonsterTime {
		t.Fatalf("a monster falls in %v, race.MonsterTime is %v", d, race.MonsterTime)
	}
}

// Every racer's game builds the same dungeon and deals the same words,
// whatever its own lists, profile and settings.
func TestRaceRunIsTheSameEverywhere(t *testing.T) {
	a, err := newRaceRun(testContext(t), testRace(), "m2")
	if err != nil {
		t.Fatal(err)
	}
	b, err := newRaceRun(&game.Context{Sound: &game.Sound{Muted: true}}, testRace(), "m3")
	if err != nil {
		t.Fatal(err)
	}
	for d := 1; d <= race.Goal; d++ {
		la, lb := a.floor(d), b.floor(d)
		if la.Start != lb.Start || !reflect.DeepEqual(la.State(), lb.State()) || la.W != race.MapSize(d) {
			t.Fatalf("floor %d differs between racers", d)
		}
		if len(la.Features) != len(lb.Features) {
			t.Fatalf("floor %d features differ", d)
		}
	}
	for range 20 {
		ea, _ := a.deck.Next()
		eb, _ := b.deck.Next()
		if !reflect.DeepEqual(ea, eb) {
			t.Fatalf("dealt %v and %v", ea, eb)
		}
	}
}

func TestRaceRunRules(t *testing.T) {
	ctx := testContext(t)
	r, err := newRaceRun(ctx, testRace(), "m2")
	if err != nil {
		t.Fatal(err)
	}
	switch {
	case !r.hardcore() || r.regen != 0:
		t.Fatal("a race isn't played by Hardcore rules")
	case r.hero.Class != rpg.Classes[0]:
		t.Fatalf("class %v", r.hero.Class)
	case r.prof != nil || r.link != nil || r.ai != nil:
		t.Fatal("a race run uses the profile, the link or the AI")
	case r.deck.Len() != 8 || r.lang.Code != "fr":
		t.Fatalf("%d words in %s, want the race list's 8 in French", r.deck.Len(), r.lang.Code)
	}
	for d := 1; d <= race.Goal; d++ {
		for _, ft := range r.floor(d).Features {
			if ft.Kind == dungeon.Shrine {
				t.Fatalf("a Save Shrine on floor %d", d)
			}
		}
	}

	c := crawlOn(r, r.floor(1))
	if slices.Contains(c.pauseItems(), pauseSuspend) || !slices.Contains(c.pauseItems(), pauseGiveUp) {
		t.Fatalf("pause items %v", c.pauseItems())
	}
	rep := r.race.report(r, c.pos)
	if rep != (race.Report{Floor: 1, X: c.pos.X, Y: c.pos.Y}) {
		t.Fatalf("report %v", rep)
	}
	r.race.monsters, r.race.fell = 2, true
	if rep := r.race.report(r, c.pos); !rep.Fell || rep.Monsters != 2 {
		t.Fatalf("report after falling %v", rep)
	}

	for _, bad := range []string{"", "title: x\nlanguage: xx\na = b\n", "title: x\nlanguage: fr\n"} {
		rc := testRace()
		rc.List = bad
		if _, err := newRaceRun(ctx, rc, "m2"); err == nil {
			t.Fatalf("raced with list %q", bad)
		}
	}
}

func TestRaceResultText(t *testing.T) {
	for _, tc := range []struct {
		r    link.Result
		want string
	}{
		{link.Result{Floor: 3, Time: 95 * time.Second, Status: link.RacerFinished}, "floor 3 in 1:35"},
		{link.Result{Floor: 2, Monsters: 1, Status: link.RacerFell}, "floor 2 · 1 monster · fell"},
		{link.Result{Floor: 2, Status: link.RacerNotCounted}, "not counted"},
	} {
		if got := resultText(tc.r); got != tc.want {
			t.Errorf("%+v: %q, want %q", tc.r, got, tc.want)
		}
	}
}
