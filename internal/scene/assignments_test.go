package scene

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/pkg/compete"
	"github.com/halpworld/halpwords/pkg/words"
)

// petsList is the assigned list the quest fixtures name.
const petsList = "title: Animals\nlanguage: fr\nid: lst_animals\nversion: 3\n\ndog = le chien\ncat = le chat\nhorse = le cheval\n"

// assignContext is a test context linked to a server that sends the
// Animals list and the quests in internal/link/testdata/assignments.json,
// as a sync would bring them.
func assignContext(t *testing.T) *game.Context {
	t.Helper()
	useTempDir(t)
	quests, err := os.ReadFile("../link/testdata/assignments.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/link":
			json.NewEncoder(w).Encode(map[string]any{"device_id": "dev_1", "token_type": "Bearer",
				"access_token": "hwd_1", "expires_in": 86400, "refresh_token": "hwr_1", "refresh_expires_in": 86400})
		case "/api/v1/lists":
			json.NewEncoder(w).Encode(map[string]any{"lists": []any{map[string]any{
				"id": "lst_animals", "version": 3, "title": "Animals", "language": "fr", "words": 3, "text": petsList}}})
		case "/api/v1/me":
			json.NewEncoder(w).Encode(map[string]any{"learner": map[string]any{"id": "lrn_1", "display_name": "Aoife"},
				"settings": map[string]any{}, "accommodations": map[string]any{}, "seen_by": []string{"guardian"},
				"game": map[string]any{"version": "1.0.0", "min_version": "1.0.0", "supported": true}})
		case "/api/v1/memory":
			json.NewEncoder(w).Encode(map[string]any{"lang": r.URL.Query().Get("lang"), "answers": 0, "memory": words.NewMemory()})
		case "/api/v1/assignments":
			w.Write(quests)
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(srv.Close)
	ctx := testContext(t)
	ctx.Link = link.Open(link.Options{Store: memFiles{}, Server: srv.URL})
	if err := ctx.Link.LinkNow(context.Background(), "ABCD-EFGH"); err != nil {
		t.Fatal(err)
	}
	if err := ctx.Link.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(ctx.Link.Quests()) != 6 || len(ctx.Link.Lists()) != 1 {
		t.Fatalf("%d quests, %d lists", len(ctx.Link.Quests()), len(ctx.Link.Lists()))
	}
	ctx.Lists = append(ctx.Lists, ctx.Link.Lists()...)
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	assignNow = func() time.Time { return now }
	t.Cleanup(func() { assignNow = time.Now })
	return ctx
}

func TestAssignmentOnTheTitleScreen(t *testing.T) {
	ctx := testContext(t)
	ti := NewTitle(ctx).(*Title)
	for _, it := range ti.items {
		if it == titleAssignments {
			t.Fatal("Assignments on the title screen of a game that isn't linked")
		}
	}
	ctx = assignContext(t)
	ti = NewTitle(ctx).(*Title)
	if ti.items[0] != titleAssignments || ti.sel != 0 {
		t.Fatalf("menu %v, chosen %d", ti.items, ti.sel)
	}
	if ti.rows() > 6 {
		t.Fatalf("%d rows don't fit", ti.rows())
	}
	// With a save too, every entry still fits, Play Together included.
	ti.hasSave = true
	ti.build(ctx)
	if ti.items[0] != titleContinue || ti.items[1] != titleAssignments || !slices.Contains(ti.items, titleTogether) || ti.rows() > 6 {
		t.Fatalf("menu with a save %v, %d rows", ti.items, ti.rows())
	}
	ti.hasSave = false
	ti.build(ctx)
	// The banner shows the quest to do next.
	if q, ok := link.Current(ti.assigns, assignNow()); !ok || q.ID != "asg_new_kind" {
		t.Fatalf("banner quest %q", q.ID)
	}
}

func TestAssignmentsScreen(t *testing.T) {
	ctx := assignContext(t)
	q := NewAssignments(ctx).(*Assignments)
	var got []string
	for _, qu := range q.quests {
		got = append(got, qu.ID+" "+qu.ProgressText())
	}
	want := "asg_new_kind 1 answer|asg_right 9/12 right|asg_master 8/20 words|asg_floor floor 2/5|asg_later not started|asg_minutes Complete!"
	if strings.Join(got, "|") != want {
		t.Fatalf("quests\n%s\nwant\n%s", strings.Join(got, "|"), want)
	}
	if q.wayText() != "◄ Practice ►" {
		t.Fatal(q.wayText())
	}

	// Practice with the quest's list only.
	p, ok := q.start(ctx).(*Practice)
	if !ok || p.assign == nil || p.assign.ID != "asg_new_kind" || len(p.deck.Entries()) != 3 || p.lang().Code != "fr" {
		t.Fatalf("practice %+v", p)
	}
	if p.langKeys() != "" || p.escTo() != "assignments" {
		t.Fatal("the practice hints offer other languages")
	}

	// An Adventure-only quest goes to the class picker, then plays its
	// list only.
	q.move(ctx, 1)
	if q.quests[q.sel].ID != "asg_right" || q.wayText() != "Adventure" {
		t.Fatalf("%s %s", q.quests[q.sel].ID, q.wayText())
	}
	cp, ok := q.start(ctx).(*ClassPick)
	if !ok || cp.setup.assign == nil || cp.setup.assign.ID != "asg_right" || setupText(cp.setup) != "Assignment: Animals" {
		t.Fatalf("class pick %+v", cp)
	}
	r := newRun(ctx, cp.lang, rpg.Knight, cp.setup)
	if r.assign == nil || len(r.deck.Entries()) != 3 || r.mode != compete.Adventure || r.linkMode() != "adventure" {
		t.Fatalf("quest run %+v", r)
	}

	// A quest whose list hasn't arrived can't start.
	q.quests[q.sel].List.ID = "lst_other"
	if q.start(ctx) != nil || q.msg == "" {
		t.Fatal("a quest without its list started")
	}

	// The campfire's look can't start a quest.
	if look := NewAssignmentsLook(ctx).(*Assignments); !look.look || len(look.quests) != 6 {
		t.Fatal("look")
	}
}

func TestAssignmentRunSaves(t *testing.T) {
	ctx := assignContext(t)
	fr, _ := words.Lookup("fr")
	r := newRun(ctx, fr, rpg.Rogue, runSetup{mode: compete.Adventure, assign: &assignRun{ID: "asg_right", List: "lst_animals", Title: "Animals"}})
	l := r.floor(1)
	data, err := encodeSave(r, l, l.Start, dungeon.North, true)
	if err != nil {
		t.Fatal(err)
	}
	s, err := decodeSave(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	if s.run.assign == nil || s.run.assign.ID != "asg_right" || len(s.run.deck.Entries()) != 3 {
		t.Fatalf("loaded %+v", s.run.assign)
	}
	// Without its list, the save explains.
	ctx.Lists = ctx.Lists[:len(ctx.Lists)-1]
	if _, err := decodeSave(ctx, data); err == nil || !strings.Contains(err.Error(), "Animals assignment") {
		t.Fatalf("no list: %v", err)
	}
}

// A save can carry a hand-made quest and an assignment quest together: the
// quest's maps are the floors, and the assignment's list is the words.
func TestSaveWithQuestAndAssignment(t *testing.T) {
	ctx := assignContext(t)
	fr, _ := words.Lookup("fr")
	q := builtInQuests()[0].q
	r := newRun(ctx, fr, rpg.Knight, runSetup{mode: compete.Adventure, quest: q,
		assign: &assignRun{ID: "asg_right", List: "lst_animals", Title: "Animals"}})
	if r.quest != q || r.assign == nil || len(r.deck.Entries()) != 3 || r.questMap(1) == nil {
		t.Fatalf("run: quest %v, assignment %v, %d words", r.quest != nil, r.assign, len(r.deck.Entries()))
	}
	l := r.floor(1)
	data, err := encodeSave(r, l, l.Start, dungeon.North, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := save.Write(saveName, data); err != nil {
		t.Fatal(err)
	}
	if s, _ := saveSummary(); s != q.Title+" · French · Knight · Floor 1 · Assignment" {
		t.Errorf("summary %q", s)
	}
	got, err := decodeSave(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	if got.run.quest == nil || got.run.quest.Title != q.Title || got.run.assign == nil || got.run.assign.ID != "asg_right" || len(got.run.deck.Entries()) != 3 {
		t.Fatalf("loaded quest %v, assignment %v", got.run.quest, got.run.assign)
	}
}

func TestDirectorIsToldTheAssignment(t *testing.T) {
	ctx := assignContext(t)
	fr, _ := words.Lookup("fr")
	// A quest run: the run's words are the quest's.
	r := newRun(ctx, fr, rpg.Knight, runSetup{mode: compete.Adventure, assign: &assignRun{ID: "asg_right", List: "lst_animals", Title: "Animals"}})
	if name, ws := r.directorAssignment(); name != "Animals" || ws != nil {
		t.Fatalf("quest run: %q %v", name, ws)
	}
	// Another Adventure in French hears about the next quest that counts
	// in an Adventure, with its words.
	r = newRun(ctx, fr, rpg.Knight, runSetup{mode: compete.Adventure})
	name, ws := r.directorAssignment()
	if name != "Animals" || len(ws) != 3 {
		t.Fatalf("adventure: %q %v", name, ws)
	}
	// Not in another language.
	la, _ := words.Lookup("la")
	if name, _ := newRun(ctx, la, rpg.Knight, runSetup{mode: compete.Adventure}).directorAssignment(); name != "" {
		t.Fatalf("Latin run told about %q", name)
	}
}
