//go:build !js

package scene

import (
	"slices"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/pkg/compete"
	"github.com/halpworld/halpwords/pkg/words"
)

// pickOnly ticks only the list with key on the checklist in lang and
// starts, as a learner would, and returns the hero scene's setup.
func pickOnly(t *testing.T, ctx *game.Context, lang *words.Language, key string) runSetup {
	t.Helper()
	lp := adventureLists(ctx, lang, runSetup{mode: compete.Adventure})
	next := ctx.TestScenes(lp)
	update(t, ctx, lp, ebiten.KeyA)
	if all, _ := lp.ticked(); len(all) > 0 {
		update(t, ctx, lp, ebiten.KeyA) // every list was ticked: A again for none
	}
	lp.sel = slices.IndexFunc(lp.rows, func(r *listRow) bool { return r.key == key })
	if lp.sel < 0 {
		t.Fatalf("no list %s", key)
	}
	update(t, ctx, lp, ebiten.KeySpace)
	update(t, ctx, lp, ebiten.KeyEnter)
	input.FakeKeys(t, func(ebiten.Key) int { return 0 })
	return next().(*ClassPick).setup
}

// selectLang selects a row of the Word Lists screen in lang, so a new
// list is typed in it.
func selectLang(w *WordLists, code string) {
	for i, r := range w.shelf.rows {
		if r.list.Language == code {
			w.sel = i
			return
		}
	}
}

// typeList types and saves a new list in the editor.
func typeList(t *testing.T, w *WordLists, ctx *game.Context, title string, lines ...string) {
	t.Helper()
	w.openEditor(ctx, nil)
	typeInEditor(w, ctx, title)
	w.ed.Enter()
	w.ed.Enter()
	for _, ln := range lines {
		typeInEditor(w, ctx, ln)
		w.ed.Enter()
	}
	w.trySaveEdit(ctx, false)
	if w.mode != wlBrowse {
		t.Fatalf("not saved: %q", w.edNote)
	}
}

func deckHas(es []words.Entry, prompt string) bool {
	return slices.ContainsFunc(es, func(e words.Entry) bool { return e.Prompt == prompt })
}

// A list typed in the editor tonight is new on the checklist: announced,
// ticked though the learner picked other lists before, first, and played
// on the first Enter. Editing it later keeps it as it was: same key, not
// new again, still ticked (#89).
func TestEditorListAppearsNew(t *testing.T) {
	w, ctx := wordListsScreen(t)
	fr, _ := words.Lookup("fr")
	starter := listKey(starterLists(fr)[0])
	pickOnly(t, ctx, fr, starter)

	selectLang(w, "fr")
	typeList(t, w, ctx, "Tonight", "dog = chien", "cat = chat")
	lp := adventureLists(ctx, fr, runSetup{mode: compete.Adventure})
	if note, _ := lp.note(); note != "New: Tonight" {
		t.Errorf("note %q", note)
	}
	if r := lp.rows[0]; r.key != "file:tonight.txt" || !r.ticked || !r.isNew {
		t.Fatalf("first row %+v", r)
	}
	next := ctx.TestScenes(lp)
	update(t, ctx, lp, ebiten.KeyEnter)
	deck := newRun(ctx, fr, rpg.Rogue, next().(*ClassPick).setup).deck.Entries()
	if !deckHas(deck, "dog") || !deckHas(deck, "cat") {
		t.Error("the first Enter did not play tonight's list")
	}

	// Edit it: a word more.
	input.FakeKeys(t, func(ebiten.Key) int { return 0 }) // Enter is let go
	for i, r := range w.shelf.rows {
		if r.file == "tonight.txt" {
			w.sel = i
			w.openEditor(ctx, r)
		}
	}
	w.ed.cur = len(w.ed.lines) - 1
	typeInEditor(w, ctx, "bird = oiseau")
	w.trySaveEdit(ctx, false)
	if !strings.Contains(userFile(t, "tonight.txt"), "bird = oiseau") {
		t.Fatalf("the edit was not saved:\n%s", userFile(t, "tonight.txt"))
	}
	lp = adventureLists(ctx, fr, runSetup{mode: compete.Adventure})
	if note, _ := lp.note(); note != "" {
		t.Errorf("new again after an edit: %q", note)
	}
	keys, _ := lp.ticked()
	if !slices.Equal(keys, []string{"file:tonight.txt", starter}) && !slices.Equal(keys, []string{starter, "file:tonight.txt"}) {
		t.Errorf("ticked %v", keys)
	}
	if n := strings.Count(strings.Join(ctx.Profile.Settings.Lists.Seen, " "), "tonight"); n != 1 {
		t.Errorf("tonight seen %d times: %v", n, ctx.Profile.Settings.Lists.Seen)
	}
	if !deckHas(newRun(ctx, fr, rpg.Rogue, runSetup{mode: compete.Adventure, pool: pickedPool(ctx, fr)}).deck.Entries(), "bird") {
		t.Error("the edited word is not played")
	}
}

// A list typed in the editor is the player's own, never the link's, even
// in a game linked to a server that sends lists.
func TestEditorListIsNeverSent(t *testing.T) {
	ctx := shelfContext(t,
		map[string]any{"id": "lst_colours", "version": 1, "title": "Colours", "language": "fr", "text": sentColours,
			"source": "sent", "sent_at": "2026-10-01T18:00:00Z"})
	withFont(t, ctx)
	ctx.Input = &input.State{}
	w := NewWordLists(ctx).(*WordLists)
	selectLang(w, "fr")
	typeList(t, w, ctx, "Colours", "red = rouge") // the same title as the sent list
	fr, _ := words.Lookup("fr")
	var own, sent *listRow
	for _, r := range listRows(ctx, fr) {
		switch r.list.Title {
		case "Colours":
			if link.IsAssigned(r.list) {
				sent = r
			} else {
				own = r
			}
		}
	}
	if own == nil || sent == nil {
		t.Fatal("both lists are not on the checklist")
	}
	if own.sent || own.assign || own.locked || own.key != "file:colours.txt" {
		t.Errorf("the typed list: %+v", own)
	}
	if li, ok := ctx.Link.Info(own.list); ok {
		t.Errorf("the typed list has link info: %+v", li)
	}
	if !sent.sent || sent.key != "lst_colours" {
		t.Errorf("the sent list: %+v", sent)
	}
}

// Daily and Hardcore runs play the built-in lists, not a starter the
// player added words to on the Word Lists screen.
func TestScoredRunsIgnoreAStarterChangedOnTheShelf(t *testing.T) {
	w, ctx := wordListsScreen(t)
	fr, _ := words.Lookup("fr")
	daily := dailySetup(ctx, fr)
	starters := entryKeys(entriesOf(starterLists(fr)))

	targets := w.shelf.targets("fr")
	i := slices.IndexFunc(targets, func(i int) bool { return w.shelf.rows[i].starter != nil || !w.shelf.rows[i].own })
	if i < 0 {
		t.Fatal("no French starter on the shelf")
	}
	w.sel = targets[i]
	w.shelf.add(w.sel, &words.List{Language: "fr", Entries: []words.Entry{{Prompt: "zebra", Answers: []string{"le zèbre"}}}})
	if !w.saveAll(ctx) {
		t.Fatal(w.msg)
	}
	file := w.shelf.rows[w.sel].file
	if _, err := save.Read(game.WordsDir + "/" + file); err != nil {
		t.Fatalf("the changed starter was not saved: %v", err)
	}
	if !deckHas(entriesFor(ctx, fr), "zebra") {
		t.Fatal("the game does not play the changed starter")
	}
	if dailySetup(ctx, fr).seed != daily.seed {
		t.Error("the changed starter changes the Daily Dungeon")
	}
	for _, setup := range []runSetup{dailySetup(ctx, fr), {mode: compete.Hardcore}} {
		if got := entryKeys(newRun(ctx, fr, rpg.Rogue, setup).deck.Entries()); !slices.Equal(got, starters) {
			t.Errorf("%v plays the changed starter", setup.mode)
		}
	}
	if !deckHas(newRun(ctx, fr, rpg.Rogue, runSetup{mode: compete.Adventure}).deck.Entries(), "zebra") {
		t.Error("an Adventure does not play the changed starter")
	}
}

// A file in the words folder the game can't read is not on the checklist,
// not even its good lines, and is not new, until it is fixed; then it is.
func TestBrokenFileNotOnTheChecklist(t *testing.T) {
	useTempUserDir(t)
	bad := "title: Bad\nlanguage: fr\ndog = chien\nthe cat chat\n"
	if err := save.Write(game.WordsDir+"/bad.txt", []byte(bad)); err != nil {
		t.Fatal(err)
	}
	ctx := testContext(t)
	if err := ctx.LoadLists(); err != nil || len(ctx.ListErrors) != 1 {
		t.Fatalf("%v %q", err, ctx.ListErrors)
	}
	fr, _ := words.Lookup("fr")
	pickOnly(t, ctx, fr, listKey(starterLists(fr)[0]))
	rows := listRows(ctx, fr)
	for _, r := range rows {
		if r.list.File == "bad.txt" || r.isNew {
			t.Errorf("row %+v", r)
		}
	}
	if ctx.Profile.Settings.Lists.WasSeen("file:bad.txt") {
		t.Error("the broken file was seen")
	}
	// Fixed, it is new.
	save.Write(game.WordsDir+"/bad.txt", []byte("title: Bad\nlanguage: fr\ndog = chien\ncat = chat\n"))
	ctx.LoadLists()
	lp := adventureLists(ctx, fr, runSetup{mode: compete.Adventure})
	if note, _ := lp.note(); note != "New: Bad" || !lp.rows[0].ticked {
		t.Errorf("note %q", note)
	}
}

// The same file imported twice as a new list is one list, so one "New".
func TestImportTwiceIsOneList(t *testing.T) {
	w, ctx := wordListsScreen(t)
	fr, _ := words.Lookup("fr")
	pickOnly(t, ctx, fr, listKey(starterLists(fr)[0]))
	data := []byte("title: Farm\nlanguage: fr\ncow = la vache\nhen = la poule\n")
	for range 2 {
		if err := w.queueFile("farm.txt", data); err != nil {
			t.Fatal(err)
		}
		w.Update(ctx)
		if w.mode != wlImport {
			t.Fatalf("mode %d", w.mode)
		}
		pressKey(t, w, ctx, ebiten.KeyEnter) // as a new list
		w.saveAll(ctx)
	}
	lp := adventureLists(ctx, fr, runSetup{mode: compete.Adventure})
	if note, _ := lp.note(); note != "New: Farm" {
		t.Errorf("note %q", note)
	}
	farms := 0
	for _, r := range lp.rows {
		if r.list.Title == "Farm" {
			farms++
		}
	}
	if farms != 1 {
		t.Errorf("%d Farm lists", farms)
	}
	// Once more, after the checklist: still one list, not new.
	w.queueFile("farm.txt", data)
	w.Update(ctx)
	pressKey(t, w, ctx, ebiten.KeyEnter)
	w.saveAll(ctx)
	lp = adventureLists(ctx, fr, runSetup{mode: compete.Adventure})
	if note, _ := lp.note(); note != "" {
		t.Errorf("note %q", note)
	}
}
