package link

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// questNow is when the quest tests look at the fixture's quests.
var questNow = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

// fixtureQuests syncs testdata/assignments.json, shaped like the server's
// GET /api/v1/assignments, through the fake server.
func fixtureQuests(t *testing.T) map[string]Quest {
	t.Helper()
	data, err := os.ReadFile("testdata/assignments.json")
	if err != nil {
		t.Fatal(err)
	}
	f, c, _, _ := linked(t)
	f.mu.Lock()
	f.quests = data
	f.mu.Unlock()
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	qs := c.Quests()
	if len(qs) != 6 {
		t.Fatalf("%d quests", len(qs))
	}
	out := map[string]Quest{}
	for _, q := range qs {
		out[q.ID] = q
	}
	// They are kept with the link.
	again := Open(c.o)
	if len(again.Quests()) != 6 {
		t.Fatal("quests weren't kept")
	}
	return out
}

func TestQuestProgressFromFixture(t *testing.T) {
	qs := fixtureQuests(t)
	for _, tc := range []struct {
		id, goal, progress, when string
		bar                      bool
		frac                     float64
		modes                    string
	}{
		{"asg_floor", "Reach floor 5 in an Adventure", "floor 2/5", "", true, 0.4, "adventure practice"},
		{"asg_master", "Master 20 words", "8/20 words", "due Fri 2 Oct", true, 0.4, "practice"},
		{"asg_right", "Spell every word right twice", "9/12 right", "due tomorrow", true, 0.75, "adventure"},
		{"asg_minutes", "Practise for 30 minutes", "Complete!", "was due 20 Sep", true, 1, "practice adventure"},
		{"asg_later", "Practise these words", "not started", "starts Mon 5 Oct", false, 0, "practice adventure"},
		// A goal kind and mode from a newer server: just practise, anywhere.
		{"asg_new_kind", "Practise these words", "1 answer", "due today", false, 0, "practice adventure"},
	} {
		q, ok := qs[tc.id]
		if !ok {
			t.Fatalf("%s missing", tc.id)
		}
		if got := q.GoalText(); got != tc.goal {
			t.Errorf("%s goal %q, want %q", tc.id, got, tc.goal)
		}
		if got := q.ProgressText(); got != tc.progress {
			t.Errorf("%s progress %q, want %q", tc.id, got, tc.progress)
		}
		if got := q.WhenText(questNow); got != tc.when {
			t.Errorf("%s when %q, want %q", tc.id, got, tc.when)
		}
		if q.HasBar() != tc.bar || (tc.bar && q.Fraction() != tc.frac) {
			t.Errorf("%s bar %v %v", tc.id, q.HasBar(), q.Fraction())
		}
		if got := strings.Join(q.PlayModes(), " "); got != tc.modes {
			t.Errorf("%s modes %q, want %q", tc.id, got, tc.modes)
		}
		if q.List.ID != "lst_animals" || q.List.Title != "Animals" || q.List.Version != 3 {
			t.Errorf("%s list %+v", tc.id, q.List)
		}
	}
	if s := qs["asg_master"].Settings; s.Accents == nil || *s.Accents != 2 || s.Timer != nil {
		t.Errorf("settings %+v", s)
	}
	if qs["asg_minutes"].Late(questNow) {
		t.Error("a complete quest is late")
	}
	if !qs["asg_right"].Late(questNow.Add(48 * time.Hour)) {
		t.Error("an overdue quest isn't late")
	}
}

func TestQuestOrder(t *testing.T) {
	qs := fixtureQuests(t)
	var all []Quest
	for _, id := range []string{"asg_floor", "asg_master", "asg_right", "asg_minutes", "asg_later", "asg_new_kind"} {
		all = append(all, qs[id])
	}
	var got []string
	for _, q := range SortQuests(all, questNow) {
		got = append(got, q.ID)
	}
	want := "asg_new_kind asg_right asg_master asg_floor asg_later asg_minutes"
	if strings.Join(got, " ") != want {
		t.Fatalf("order %v\nwant  %s", got, want)
	}
	if q, ok := Current(all, questNow); !ok || q.ID != "asg_new_kind" {
		t.Fatalf("current %v %v", q.ID, ok)
	}
	if _, ok := Current([]Quest{qs["asg_minutes"], qs["asg_later"]}, questNow); ok {
		t.Fatal("a complete or later quest is current")
	}
	// Due dates are shown in the player's time zone.
	nz := time.FixedZone("NZDT", 13*3600)
	if got := qs["asg_master"].WhenText(questNow.In(nz)); got != "due Sat 3 Oct" {
		t.Errorf("in New Zealand %q", got)
	}
}
