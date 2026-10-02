package scene

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/assets"
	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/internal/unifont"
	"github.com/halpworld/halpwords/pkg/compete"
	"github.com/halpworld/halpwords/pkg/words"
)

// ownList is one of the player's own lists, as game.UserLists reads it.
func ownList(t *testing.T, file, src string) *words.List {
	t.Helper()
	l, err := words.Parse(strings.NewReader(src), file)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// eight is a small list of the player's own, for one evening.
const eight = "title: Tonight\nlanguage: fr\n\nred = rouge\nblue = bleu\ngreen = vert\nblack = noir\nwhite = blanc\nyellow = jaune\npink = rose\ngrey = gris\n"

// threeWords is smaller still.
const threeWords = "title: Three\nlanguage: fr\n\none = un\ntwo = deux\nthree = trois\n"

func entryKeys(es []words.Entry) []string {
	var out []string
	for _, e := range es {
		out = append(out, words.Key(e))
	}
	slices.Sort(out)
	return out
}

func TestListPoolLists(t *testing.T) {
	ctx := testContext(t)
	fr, _ := words.Lookup("fr")
	tonight := ownList(t, "tonight.txt", eight)
	ctx.Lists = append(ctx.Lists, tonight)
	all := ctx.ListsFor("fr")

	if got := (listPool{}).lists(ctx, fr); !slices.Equal(got, all) {
		t.Error("no keys is not every list")
	}
	got := listPool{keys: []string{"file:tonight.txt"}}.lists(ctx, fr)
	if len(got) != 1 || got[0] != tonight {
		t.Errorf("one list ticked: %v", got)
	}
	// The game's order, whatever the order ticked; unknown keys go.
	keys := []string{"file:tonight.txt", "gone", listKey(all[0])}
	got = listPool{keys: keys}.lists(ctx, fr)
	if len(got) != 2 || got[0] != all[0] || got[1] != tonight {
		t.Errorf("order: %v", got)
	}
	// Every list ticked is gone: every list, rather than none.
	if got := (listPool{keys: []string{"gone"}}).lists(ctx, fr); !slices.Equal(got, all) {
		t.Error("lists that are gone left no words")
	}
}

// Daily and Hardcore runs play the built-in lists only: not the
// player's own, and not their edited copy of a starter (#89).
func TestScoredRunsPlayTheStarters(t *testing.T) {
	ctx := testContext(t)
	fr, _ := words.Lookup("fr")
	daily := dailySetup(ctx, fr)
	starters := entryKeys(entriesOf(starterLists(fr)))

	// An edited copy of the French starter replaces it in the game, and
	// an own list joins it.
	starter := ctx.ListsFor("fr")[0]
	edited := ownList(t, starter.File, "title: Mine\nlanguage: fr\n\ncat = le chat\n")
	for i, l := range ctx.Lists {
		if l == starter {
			ctx.Lists[i] = edited
		}
	}
	ctx.Lists = append(ctx.Lists, ownList(t, "tonight.txt", eight))
	if again := dailySetup(ctx, fr); again.seed != daily.seed {
		t.Error("the player's lists change the Daily Dungeon")
	}
	for _, setup := range []runSetup{daily, {mode: compete.Hardcore}, {mode: compete.Hardcore, seed: 7, seeded: true}} {
		setup.pool = listPool{keys: []string{"file:tonight.txt"}} // ignored
		r := newRun(ctx, fr, rpg.Rogue, setup)
		if got := entryKeys(r.deck.Entries()); !slices.Equal(got, starters) {
			t.Errorf("%v run plays %d words, not the %d starters", setup.mode, len(got), len(starters))
		}
	}
	// An Adventure plays what was ticked.
	r := newRun(ctx, fr, rpg.Rogue, runSetup{mode: compete.Adventure, pool: listPool{keys: []string{"file:tonight.txt"}}})
	if r.deck.Len() != 8 {
		t.Errorf("an Adventure on the 8-word list deals from %d words", r.deck.Len())
	}
}

// The ticked lists and the built-in-only rule are kept in the save, so
// a run loads with the words it was made with; saves from before have
// neither and play every list, as they were made.
func TestSaveKeepsTheLists(t *testing.T) {
	ctx := testContext(t)
	fr, _ := words.Lookup("fr")
	ctx.Lists = append(ctx.Lists, ownList(t, "tonight.txt", eight))
	for _, setup := range []runSetup{
		{mode: compete.Adventure, pool: listPool{keys: []string{"file:tonight.txt"}}},
		{mode: compete.Hardcore},
	} {
		r := newRun(ctx, fr, rpg.Knight, setup)
		l := r.floor(1)
		data, err := encodeSave(r, l, l.Start, dungeon.North, true)
		if err != nil {
			t.Fatal(err)
		}
		got, err := decodeSave(ctx, data)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.run.deck.Entries(), r.deck.Entries()) || !reflect.DeepEqual(got.run.pool, r.pool) {
			t.Errorf("%v: the save came back with %d words, not %d", setup.mode, got.run.deck.Len(), r.deck.Len())
		}
	}
	// A save from before: every list.
	r := newRun(ctx, fr, rpg.Knight, runSetup{mode: compete.Adventure})
	l := r.floor(1)
	data, _ := encodeSave(r, l, l.Start, dungeon.North, true)
	if strings.Contains(string(data), `"Lists"`) || strings.Contains(string(data), `"Starters"`) {
		t.Errorf("a run on every list writes its lists: %s", data)
	}
	got, err := decodeSave(ctx, data)
	if err != nil || got.run.deck.Len() != len(entriesFor(ctx, fr)) {
		t.Fatalf("an old save: %v", err)
	}
}

// update runs one Update of s with only key pressed.
func update(t *testing.T, ctx *game.Context, s game.Scene, key ebiten.Key) {
	t.Helper()
	press(t, key)
	if err := s.Update(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestChecklist(t *testing.T) {
	ctx := testContext(t)
	ctx.Input = &input.State{}
	fr, _ := words.Lookup("fr")
	ctx.Lists = append(ctx.Lists, ownList(t, "tonight.txt", eight))
	lp := adventureLists(ctx, fr, runSetup{mode: compete.Adventure})
	next := ctx.TestScenes(lp)
	if len(lp.rows) != len(ctx.ListsFor("fr")) {
		t.Fatalf("%d rows", len(lp.rows))
	}
	for _, r := range lp.rows {
		if !r.ticked {
			t.Fatal("never chosen: every list is ticked")
		}
	}
	// A: none. Enter can't start.
	update(t, ctx, lp, ebiten.KeyA)
	if _, n := lp.ticked(); n != 0 {
		t.Fatalf("A left %d words", n)
	}
	update(t, ctx, lp, ebiten.KeyEnter)
	if next() != nil || lp.msg == "" {
		t.Fatal("started with nothing ticked")
	}
	// Tick only Tonight, and start.
	i := slices.IndexFunc(lp.rows, func(r *listRow) bool { return r.key == "file:tonight.txt" })
	lp.sel = i
	update(t, ctx, lp, ebiten.KeySpace)
	if keys, n := lp.ticked(); n != 8 || !slices.Equal(keys, []string{"file:tonight.txt"}) {
		t.Fatalf("ticked %v (%d words)", keys, n)
	}
	update(t, ctx, lp, ebiten.KeyEnter)
	pick, ok := next().(*ClassPick)
	if !ok || !pick.setup.picked || !slices.Equal(pick.setup.pool.keys, []string{"file:tonight.txt"}) {
		t.Fatalf("Enter led to %T", pick)
	}
	if keys, ok := ctx.Profile.Settings.Lists.Picked("fr"); !ok || !slices.Equal(keys, []string{"file:tonight.txt"}) {
		t.Errorf("remembered %v", keys)
	}
	// Esc on the hero goes back to the checklist.
	update(t, ctx, pick, ebiten.KeyEscape)
	lp, ok = next().(*ListPick)
	if !ok {
		t.Fatal("Esc on the hero did not go back to the lists")
	}

	// Next time, one key: Enter plays the same lists.
	update(t, ctx, lp, ebiten.KeyEnter)
	pick = next().(*ClassPick)
	if !slices.Equal(pick.setup.pool.keys, []string{"file:tonight.txt"}) {
		t.Errorf("the second time played %v", pick.setup.pool.keys)
	}
	r := newRun(ctx, fr, rpg.Knight, pick.setup)
	if r.deck.Len() != 8 {
		t.Errorf("the run deals from %d words", r.deck.Len())
	}
	// Practice remembers it too.
	if p := NewPractice(ctx).(*Practice); p.deck.Len() != 8 {
		t.Errorf("practice deals from %d words", p.deck.Len())
	}

	// Ticking every list forgets the pick, so lists added later are
	// ticked too.
	lp = adventureLists(ctx, fr, runSetup{mode: compete.Adventure})
	next = ctx.TestScenes(lp)
	update(t, ctx, lp, ebiten.KeyA)
	update(t, ctx, lp, ebiten.KeyEnter)
	next()
	if _, ok := ctx.Profile.Settings.Lists.Picked("fr"); ok {
		t.Error("every list ticked is kept as a pick")
	}
}

// Lists that are gone are dropped from the pick; the rest stay ticked.
func TestChecklistDropsListsThatAreGone(t *testing.T) {
	ctx := testContext(t)
	fr, _ := words.Lookup("fr")
	ctx.Lists = append(ctx.Lists, ownList(t, "tonight.txt", eight))
	ctx.Profile.Settings.Lists.Pick("fr", []string{"lst_gone", "file:tonight.txt"})
	lp := adventureLists(ctx, fr, runSetup{mode: compete.Adventure})
	if keys, n := lp.ticked(); !slices.Equal(keys, []string{"file:tonight.txt"}) || n != 8 {
		t.Errorf("ticked %v", keys)
	}
}

func TestChecklistWarnsOfFewWords(t *testing.T) {
	ctx := testContext(t)
	ctx.Input = &input.State{}
	fr, _ := words.Lookup("fr")
	ctx.Lists = append(ctx.Lists, ownList(t, "three.txt", threeWords))
	ctx.Profile.Settings.Lists.Pick("fr", []string{"file:three.txt"})
	lp := adventureLists(ctx, fr, runSetup{mode: compete.Adventure})
	if _, n := lp.ticked(); n != 3 || n >= fewWords {
		t.Fatalf("%d words", n)
	}
	// It draws, with the warning, at the game's size, and the lines fit.
	face, err := unifont.ParseBytes(assets.UnifontHex)
	if err != nil {
		t.Fatal(err)
	}
	ctx.Font = gfx.NewFont(face)
	for _, s := range []string{"Only a few words: they will come round often.", "Choose your word lists"} {
		if w := ctx.Font.Width(s, 1); w > lpW-8 {
			t.Errorf("%q is %dpx", s, w)
		}
	}
	lp.Draw(ebiten.NewImage(game.ScreenW, game.ScreenH), ctx)
}

// Which runs ask for lists: Adventures and quests that name none.
func TestPickNeeded(t *testing.T) {
	ctx := testContext(t)
	fr, _ := words.Lookup("fr")
	for _, c := range []struct {
		setup runSetup
		want  bool
	}{
		{runSetup{mode: compete.Adventure}, true},
		{runSetup{mode: compete.Adventure, quest: builtInQuests()[0].q}, true},
		{runSetup{mode: compete.Hardcore}, false},
		{runSetup{mode: compete.Daily}, false},
		{runSetup{mode: compete.Adventure, assign: &assignRun{ID: "asg_1"}}, false},
	} {
		if got := pickNeeded(ctx, fr, c.setup); got != c.want {
			t.Errorf("%+v: %v", c.setup, got)
		}
	}
	// The language picker of a Daily goes straight to the hero.
	ctx.Input = &input.State{}
	a := NewAdventure(ctx, runSetup{mode: compete.Daily})
	next := ctx.TestScenes(a)
	update(t, ctx, a, ebiten.KeyEnter)
	if _, ok := next().(*ClassPick); !ok {
		t.Error("a Daily asked for lists")
	}
}

func TestPracticeChecklist(t *testing.T) {
	ctx := testContext(t)
	ctx.Input = &input.State{}
	ctx.Lists = append(ctx.Lists, ownList(t, "three.txt", threeWords))
	lp := NewPracticeLists(ctx).(*ListPick)
	next := ctx.TestScenes(lp)
	first := lp.lang
	update(t, ctx, lp, ebiten.KeyArrowRight)
	if lp.lang == first {
		t.Fatal("→ did not change the language")
	}
	update(t, ctx, lp, ebiten.KeyArrowLeft)
	if lp.lang.Code != "fr" {
		t.Fatalf("back to %s", lp.lang.Code)
	}
	update(t, ctx, lp, ebiten.KeyA) // none
	lp.sel = slices.IndexFunc(lp.rows, func(r *listRow) bool { return r.key == "file:three.txt" })
	update(t, ctx, lp, ebiten.KeySpace)
	update(t, ctx, lp, ebiten.KeyEnter)
	p, ok := next().(*Practice)
	if !ok || p.lang().Code != "fr" || p.deck.Len() != 3 {
		t.Fatalf("practice: %T", p)
	}
	// Three words deal without a word twice in a row.
	last := ""
	for range 60 {
		e, id := p.deck.Next()
		if e.Prompt == last {
			t.Fatalf("%q twice in a row", last)
		}
		last = e.Prompt
		p.deck.Mark(id, id != 0)
	}
}

// shelfContext is a test context linked to a server whose shelf has
// lists (GET /api/v1/lists bodies, with the #89 fields).
func shelfContext(t *testing.T, lists ...map[string]any) *game.Context {
	t.Helper()
	useTempDir(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/link":
			json.NewEncoder(w).Encode(map[string]any{"device_id": "dev_1", "token_type": "Bearer",
				"access_token": "hwd_1", "expires_in": 86400, "refresh_token": "hwr_1", "refresh_expires_in": 86400})
		case "/api/v1/lists":
			json.NewEncoder(w).Encode(map[string]any{"lists": lists})
		case "/api/v1/me":
			json.NewEncoder(w).Encode(map[string]any{"learner": map[string]any{"id": "lrn_1", "display_name": "Aoife"},
				"settings": map[string]any{}, "accommodations": map[string]any{}, "seen_by": []string{"guardian"},
				"game": map[string]any{"version": "1.0.0", "min_version": "1.0.0", "supported": true}})
		case "/api/v1/memory":
			json.NewEncoder(w).Encode(map[string]any{"lang": r.URL.Query().Get("lang"), "answers": 0, "memory": words.NewMemory()})
		case "/api/v1/assignments":
			w.Write([]byte(`{"assignments":[]}`))
		case "/api/v1/runs", "/api/v1/ranks": // no rankings on this server
			w.WriteHeader(http.StatusNotFound)
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
	ctx.Lists = append(ctx.Lists, ctx.Link.Lists()...)
	return ctx
}

const (
	sentColours = "title: Colours\nlanguage: fr\nid: lst_colours\nversion: 1\n\nred = rouge\nblue = bleu\n"
	sentNumbers = "title: Numbers\nlanguage: fr\nid: lst_numbers\nversion: 1\n\none = un\ntwo = deux\n"
)

// A newly sent list is ticked once, and said to be new; after that the
// learner may untick it like any other (#89).
func TestSentListIsTickedOnce(t *testing.T) {
	ctx := shelfContext(t,
		map[string]any{"id": "lst_colours", "version": 1, "title": "Colours", "language": "fr", "text": sentColours,
			"source": "sent", "sent_at": "2026-10-01T18:00:00Z"},
		map[string]any{"id": "lst_numbers", "version": 1, "title": "Numbers", "language": "fr", "text": sentNumbers,
			"source": "sent", "sent_at": "2026-10-02T18:00:00Z"})
	ctx.Input = &input.State{}
	fr, _ := words.Lookup("fr")
	starter := listKey(starterLists(fr)[0])
	ctx.Profile.Settings.Lists.Pick("fr", []string{starter})

	lp := adventureLists(ctx, fr, runSetup{mode: compete.Adventure})
	if lp.rows[0].key != "lst_numbers" || lp.rows[1].key != "lst_colours" {
		t.Errorf("not newest first: %s, %s", lp.rows[0].key, lp.rows[1].key)
	}
	if note, _ := lp.note(); note != "New: Numbers" && note != "New: Colours" {
		t.Errorf("note %q", note)
	}
	keys, _ := lp.ticked()
	if !slices.Contains(keys, "lst_colours") || !slices.Contains(keys, "lst_numbers") || !slices.Contains(keys, starter) {
		t.Fatalf("ticked %v", keys)
	}
	if !ctx.Profile.Settings.Lists.WasSeen("lst_colours") || !ctx.Profile.Settings.Lists.WasSeen("lst_numbers") {
		t.Error("the sent lists were not marked seen")
	}
	// The learner unticks one, and it stays unticked.
	next := ctx.TestScenes(lp)
	lp.sel = 0
	update(t, ctx, lp, ebiten.KeySpace)
	update(t, ctx, lp, ebiten.KeyEnter)
	next()
	lp = adventureLists(ctx, fr, runSetup{mode: compete.Adventure})
	if note, _ := lp.note(); note != "" {
		t.Errorf("still new: %q", note)
	}
	if keys, _ := lp.ticked(); slices.Contains(keys, "lst_numbers") || !slices.Contains(keys, "lst_colours") {
		t.Errorf("ticked %v", keys)
	}
}

// While an assignment locks a list, Adventure and Practice play only the
// locked lists, and the learner can't untick them (#89).
func TestLockedLists(t *testing.T) {
	ctx := shelfContext(t,
		map[string]any{"id": "lst_colours", "version": 1, "title": "Colours", "language": "fr", "text": sentColours,
			"source": "assignment", "locked": true},
		map[string]any{"id": "lst_numbers", "version": 1, "title": "Numbers", "language": "fr", "text": sentNumbers,
			"source": "sent", "sent_at": "2026-10-02T18:00:00Z"})
	ctx.Input = &input.State{}
	fr, _ := words.Lookup("fr")
	lp := adventureLists(ctx, fr, runSetup{mode: compete.Adventure})
	next := ctx.TestScenes(lp)
	if !lp.locked {
		t.Fatal("not locked")
	}
	if keys, _ := lp.ticked(); !slices.Equal(keys, []string{"lst_colours"}) {
		t.Fatalf("ticked %v", keys)
	}
	update(t, ctx, lp, ebiten.KeyA)
	lp.sel = 0
	update(t, ctx, lp, ebiten.KeySpace)
	if keys, _ := lp.ticked(); !slices.Equal(keys, []string{"lst_colours"}) || lp.msg == "" {
		t.Fatalf("the lock gave way: %v", keys)
	}
	update(t, ctx, lp, ebiten.KeyEnter)
	pick := next().(*ClassPick)
	if r := newRun(ctx, fr, rpg.Knight, pick.setup); r.deck.Len() != 2 {
		t.Errorf("the run deals from %d words", r.deck.Len())
	}
	if p := NewPractice(ctx).(*Practice); p.deck.Len() != 2 {
		t.Errorf("practice deals from %d words", p.deck.Len())
	}
	// The learner's own pick is left for when the lock goes.
	if _, ok := ctx.Profile.Settings.Lists.Picked("fr"); ok {
		t.Error("the lock was kept as the learner's pick")
	}
	// Other languages are not locked.
	la, _ := words.Lookup("la")
	if lockedLists(ctx, la) != nil {
		t.Error("Latin is locked")
	}
}

// The checklist draws nothing from a run's random numbers: the same
// lists and seed make the same dungeon and deal the same words, however
// the lists were ticked.
func TestChecklistKeepsRunsDeterministic(t *testing.T) {
	ctx := testContext(t)
	ctx.Input = &input.State{}
	fr, _ := words.Lookup("fr")
	ctx.Lists = append(ctx.Lists, ownList(t, "tonight.txt", eight), ownList(t, "three.txt", threeWords))
	setup := runSetup{mode: compete.Adventure, seed: 42, seeded: true}
	lp := adventureLists(ctx, fr, setup)
	next := ctx.TestScenes(lp)
	update(t, ctx, lp, ebiten.KeyA)
	for i, r := range lp.rows { // tick in the other order from the game's
		if r.key == "file:three.txt" || r.key == "file:tonight.txt" {
			lp.sel = i
			update(t, ctx, lp, ebiten.KeySpace)
		}
	}
	update(t, ctx, lp, ebiten.KeyEnter)
	viaUI := newRun(ctx, fr, rpg.Rogue, next().(*ClassPick).setup)
	setup.pool = listPool{keys: []string{"file:tonight.txt", "file:three.txt"}}
	direct := newRun(ctx, fr, rpg.Rogue, setup)
	if viaUI.floorSeed(1) != direct.floorSeed(1) || !reflect.DeepEqual(viaUI.deck.Entries(), direct.deck.Entries()) {
		t.Fatal("the checklist changed the run")
	}
	for range 20 {
		a, _ := viaUI.deck.Next()
		b, _ := direct.deck.Next()
		if a.Prompt != b.Prompt {
			t.Fatalf("dealt %q and %q", a.Prompt, b.Prompt)
		}
	}
}
