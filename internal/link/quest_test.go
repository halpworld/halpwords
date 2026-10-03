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
	// The due date is the server's UTC date, even in New Zealand.
	nz := time.FixedZone("NZDT", 13*3600)
	if got := qs["asg_master"].WhenText(questNow.In(nz)); got != "due Fri 2 Oct" {
		t.Errorf("in New Zealand %q", got)
	}
}

// A due date is the UTC calendar date the server and the website show
// (the server sends the last moment of that day in UTC), east or west
// of UTC; "today" is the player's own date (F-U3-01: east of UTC, the
// game showed the next day).
func TestDueDateIsTheUTCDate(t *testing.T) {
	sydney := time.FixedZone("AEDT", 11*3600)
	dublin := time.FixedZone("IST", 3600) // Dublin in summer
	newYork := time.FixedZone("EST", -5*3600)
	xmas := Quest{DueAt: "2026-12-24T23:59:59.999Z"}
	july := Quest{DueAt: "2026-07-10T23:59:59.999Z"}
	for _, tc := range []struct {
		name string
		q    Quest
		now  time.Time
		when string
		late bool
	}{
		{"Sydney, days before", xmas, time.Date(2026, 12, 20, 12, 0, 0, 0, sydney), "due Thu 24 Dec", false},
		{"Sydney, on the day", xmas, time.Date(2026, 12, 24, 12, 0, 0, 0, sydney), "due today", false},
		// Still 24 Dec in UTC, but 25 Dec in Sydney: the day has gone.
		{"Sydney, the day after", xmas, time.Date(2026, 12, 25, 9, 0, 0, 0, sydney), "was due 24 Dec", true},
		{"Dublin in summer", july, time.Date(2026, 7, 8, 12, 0, 0, 0, dublin), "due Fri 10 Jul", false},
		{"Dublin, just after midnight", july, time.Date(2026, 7, 11, 0, 30, 0, 0, dublin), "was due 10 Jul", true},
		{"New York, the day before", xmas, time.Date(2026, 12, 23, 12, 0, 0, 0, newYork), "due tomorrow", false},
		// Already 25 Dec in UTC, but still the due day in New York.
		{"New York, the evening of the day", xmas, time.Date(2026, 12, 24, 20, 0, 0, 0, newYork), "due today", false},
		{"New York, a start date", Quest{StartsAt: "2026-12-21T00:00:00Z", DueAt: xmas.DueAt},
			time.Date(2026, 12, 20, 12, 0, 0, 0, newYork), "starts tomorrow", false}, // not "today"
	} {
		if got := tc.q.WhenText(tc.now); got != tc.when {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.when)
		}
		if got := tc.q.Late(tc.now); got != tc.late {
			t.Errorf("%s: late %v", tc.name, got)
		}
	}
}
