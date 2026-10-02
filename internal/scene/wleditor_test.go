//go:build !js

package scene

import (
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/save"
)

func wordListsScreen(t *testing.T) (*WordLists, *game.Context) {
	t.Helper()
	useTempUserDir(t)
	ctx := testContext(t)
	withFont(t, ctx)
	ctx.Input = &input.State{}
	if err := ctx.LoadLists(); err != nil {
		t.Fatal(err)
	}
	return NewWordLists(ctx).(*WordLists), ctx
}

// typeInEditor types s into the open editor, one tick.
func typeInEditor(w *WordLists, ctx *game.Context, s string) {
	ctx.Input.Chars = []rune(s)
	w.updateEdit(ctx)
	ctx.Input.Chars = nil
}

// press holds only key for one update of w.
func pressKey(t *testing.T, w *WordLists, ctx *game.Context, key ebiten.Key) {
	t.Helper()
	holdKey(t, key)
	w.Update(ctx)
	input.FakeKeys(t, func(ebiten.Key) int { return 0 })
}

func userFile(t *testing.T, name string) string {
	t.Helper()
	b, err := save.Read(game.WordsDir + "/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A parent types a list in the game and it is saved in the learner's words
// folder, and the game uses it.
func TestEditorSavesNewList(t *testing.T) {
	w, ctx := wordListsScreen(t)
	before := len(ctx.Lists)
	w.openEditor(ctx, nil)
	if w.mode != wlEdit {
		t.Fatal("the editor did not open")
	}
	typeInEditor(w, ctx, "Tonight's homework") // title
	w.ed.Enter()
	w.ed.Language(0)
	w.ed.Enter()
	for _, ln := range []string{"dog = chien", "cat = chat", "bird = oiseau | l'oiseau"} {
		typeInEditor(w, ctx, ln)
		w.ed.Enter()
	}
	w.trySaveEdit(ctx, false)
	if w.mode != wlBrowse || w.edAsk != edAskNone {
		t.Fatalf("mode %d ask %d note %q", w.mode, w.edAsk, w.edNote)
	}
	file := "tonight-s-homework.txt"
	got := userFile(t, file)
	for _, want := range []string{"title: Tonight's homework", "dog = chien", "bird = oiseau | l'oiseau"} {
		if !strings.Contains(got, want) {
			t.Errorf("file lacks %q:\n%s", want, got)
		}
	}
	if len(ctx.Lists) != before+1 {
		t.Errorf("game has %d lists, want %d", len(ctx.Lists), before+1)
	}
	if r := w.shelf.rows[w.sel]; r.file != file || len(r.list.Entries) != 3 {
		t.Errorf("selected %+v", r)
	}
}

// Lines that are not words are left out only after the typist agrees.
func TestEditorAsksBeforeLeavingLinesOut(t *testing.T) {
	w, ctx := wordListsScreen(t)
	w.openEditor(ctx, nil)
	typeInEditor(w, ctx, "Pets")
	w.ed.Enter()
	w.ed.Enter()
	typeInEditor(w, ctx, "dog = chien")
	w.ed.Enter()
	typeInEditor(w, ctx, "just words")
	w.trySaveEdit(ctx, false)
	if w.mode != wlEdit || w.edAsk != edAskSkip {
		t.Fatalf("mode %d ask %d", w.mode, w.edAsk)
	}
	if _, err := save.Read(game.WordsDir + "/pets.txt"); err == nil {
		t.Fatal("saved before they agreed")
	}
	pressKey(t, w, ctx, ebiten.KeyEscape)
	if w.edAsk != edAskNone || w.mode != wlEdit {
		t.Fatalf("Esc: mode %d ask %d", w.mode, w.edAsk)
	}
	w.trySaveEdit(ctx, false)
	pressKey(t, w, ctx, ebiten.KeyY)
	if w.mode != wlBrowse || strings.Contains(userFile(t, "pets.txt"), "just words") {
		t.Errorf("mode %d:\n%s", w.mode, userFile(t, "pets.txt"))
	}
}

func TestEditorNeedsTitleAndWords(t *testing.T) {
	w, ctx := wordListsScreen(t)
	w.openEditor(ctx, nil)
	w.trySaveEdit(ctx, false)
	if w.edNote == "" || w.mode != wlEdit {
		t.Errorf("empty list: note %q mode %d", w.edNote, w.mode)
	}
	typeInEditor(w, ctx, "x")
	if w.edNote != "" {
		t.Error("the note stayed after typing")
	}
}

func TestEditorEscAsksOnlyWhenChanged(t *testing.T) {
	w, ctx := wordListsScreen(t)
	w.openEditor(ctx, nil)
	pressKey(t, w, ctx, ebiten.KeyEscape)
	if w.mode != wlBrowse {
		t.Errorf("an untouched editor stayed open (mode %d)", w.mode)
	}
	w.openEditor(ctx, nil)
	typeInEditor(w, ctx, "x")
	pressKey(t, w, ctx, ebiten.KeyEscape)
	if w.mode != wlEdit || w.edAsk != edAskLeave {
		t.Errorf("mode %d ask %d", w.mode, w.edAsk)
	}
	pressKey(t, w, ctx, ebiten.KeyN)
	if w.mode != wlBrowse {
		t.Error("N did not throw the list away")
	}
	if _, err := save.Read(game.WordsDir + "/x.txt"); err == nil {
		t.Error("a thrown-away list was saved")
	}
}

// Your own lists can be edited; starter and assigned lists can't.
func TestEditExistingList(t *testing.T) {
	w, ctx := wordListsScreen(t)
	w.openEditor(ctx, w.shelf.rows[0]) // a starter list
	if w.mode != wlBrowse {
		t.Fatal("a starter list opened in the editor")
	}
	w.openEditor(ctx, nil)
	typeInEditor(w, ctx, "Mine")
	w.ed.Enter()
	w.ed.Enter()
	typeInEditor(w, ctx, "dog = chien")
	w.trySaveEdit(ctx, false)

	row := w.shelf.rows[w.sel]
	w.openEditor(ctx, row)
	if w.mode != wlEdit || w.edRow != row || !strings.Contains(w.ed.text(), "dog = chien") {
		t.Fatalf("mode %d, text %q", w.mode, w.ed.text())
	}
	w.ed.Enter()
	typeInEditor(w, ctx, "cat = chat")
	w.trySaveEdit(ctx, false)
	got := userFile(t, "mine.txt")
	if !strings.Contains(got, "cat = chat") || !strings.Contains(got, "dog = chien") {
		t.Errorf("file:\n%s", got)
	}
	if n := strings.Count(got, "title:"); n != 1 {
		t.Errorf("%d title lines:\n%s", n, got)
	}
	for _, r := range w.shelf.rows {
		if r.file == "mine.txt" && len(r.list.Entries) != 2 {
			t.Errorf("row has %d words", len(r.list.Entries))
		}
	}

	assigned := &shelfRow{file: "a.txt", list: row.list, assigned: true}
	w.openEditor(ctx, assigned)
	if w.mode != wlBrowse {
		t.Error("an assigned list opened in the editor")
	}
}

// A file in the words folder with a bad line does not stop the game loading:
// the Word Lists screen warns about it and the editor can fix it.
func TestBrokenFolderFile(t *testing.T) {
	useTempUserDir(t)
	good := "title: Good\nlanguage: fr\ndog = chien\n"
	bad := "title: Bad\nlanguage: fr\ndog = chien\nthe cat chat\n\ncat = chat\n"
	for n, s := range map[string]string{"good.txt": good, "bad.txt": bad, "nolang.txt": "dog = chien\n"} {
		if err := save.Write(game.WordsDir+"/"+n, []byte(s)); err != nil {
			t.Fatal(err)
		}
	}
	ctx := testContext(t)
	withFont(t, ctx)
	ctx.Input = &input.State{}
	if err := ctx.LoadLists(); err != nil {
		t.Fatalf("the game did not load: %v", err)
	}
	found := false
	for _, l := range ctx.Lists {
		found = found || l.File == "good.txt"
	}
	if !found || len(ctx.ListErrors) != 2 {
		t.Fatalf("good list loaded: %v, errors %q", found, ctx.ListErrors)
	}

	w := NewWordLists(ctx).(*WordLists)
	if n := w.shelf.brokenCount(); n != 2 || !strings.HasPrefix(w.msg, "! 2 files") {
		t.Fatalf("broken rows %d, message %q", n, w.msg)
	}
	var row *shelfRow
	for i, r := range w.shelf.rows {
		if r.file == "bad.txt" {
			row, w.sel = r, i
		}
	}
	if row == nil || len(row.broken) != 1 || row.broken[0].Line != 4 || len(row.list.Entries) == 0 {
		t.Fatalf("row %+v", row)
	}
	// It can't be an import target, and W shows the problems.
	for _, i := range w.shelf.targets("fr") {
		if w.shelf.rows[i].broken != nil {
			t.Error("a broken file is an import target")
		}
	}
	pressKey(t, w, ctx, ebiten.KeyW)
	if w.mode != wlPreview || w.pvRow != row {
		t.Fatalf("mode %d", w.mode)
	}
	// Enter goes to the editor on the raw text, and fixing it saves a good file.
	pressKey(t, w, ctx, ebiten.KeyEnter)
	if w.mode != wlEdit || w.ed.Problem(2) == "" {
		t.Fatalf("mode %d, text %q", w.mode, w.ed.text())
	}
	w.ed.cur = 1
	for w.ed.lines[1].Len() > 0 {
		w.ed.Backspace()
	}
	typeInEditor(w, ctx, "bird = oiseau")
	w.trySaveEdit(ctx, false)
	if w.mode != wlBrowse {
		t.Fatalf("mode %d note %q", w.mode, w.edNote)
	}
	if _, errs := game.UserLists(); len(errs) != 1 { // nolang.txt is left
		t.Errorf("errors after fixing: %q", errs)
	}
	if w.shelf.brokenCount() != 1 {
		t.Errorf("%d broken rows after fixing", w.shelf.brokenCount())
	}
}

// Nothing is imported silently: a file with bad lines shows what was skipped
// first, Esc imports nothing, and Enter imports the good lines.
func TestImportPreview(t *testing.T) {
	w, ctx := wordListsScreen(t)
	rows := len(w.shelf.rows)
	data := []byte("title: Farm\nlanguage: fr\r\ncow = la vache\r\npig\r\nhen = la poule\r\nduck = \r\n")
	if err := w.queueFile("farm.txt", data); err != nil {
		t.Fatal(err)
	}
	w.Update(ctx)
	if w.mode != wlPreview || len(w.pvProbs) != 2 || len(w.imp.Entries) != 2 {
		t.Fatalf("mode %d, problems %v", w.mode, w.pvProbs)
	}
	lines := w.pvLines(ctx)
	if len(lines) < 4 || lines[0].s != "line 4: pig" {
		t.Errorf("lines %+v", lines)
	}
	pressKey(t, w, ctx, ebiten.KeyEscape)
	if w.mode != wlBrowse || len(w.shelf.rows) != rows {
		t.Fatalf("Esc: mode %d, rows %d", w.mode, len(w.shelf.rows))
	}

	w.queueFile("farm.txt", data)
	w.Update(ctx)
	pressKey(t, w, ctx, ebiten.KeyEnter) // import the good lines
	if w.mode != wlImport || len(w.shelf.rows) != rows {
		t.Fatalf("Enter: mode %d, rows %d", w.mode, len(w.shelf.rows))
	}
	pressKey(t, w, ctx, ebiten.KeyEnter) // as a new list
	if len(w.shelf.rows) != rows+1 || len(w.shelf.rows[w.sel].list.Entries) != 2 {
		t.Errorf("rows %d", len(w.shelf.rows))
	}

	// A clean file goes straight to choosing where it goes.
	w.queueFile("ok.txt", []byte("language: fr\nsun = le soleil\n"))
	w.Update(ctx)
	if w.mode != wlImport {
		t.Errorf("clean file: mode %d", w.mode)
	}
	pressKey(t, w, ctx, ebiten.KeyEscape)

	// A file with nothing usable can be looked at, not imported.
	w.queueFile("junk.txt", []byte("hello\nworld\n"))
	w.Update(ctx)
	if w.mode != wlPreview {
		t.Fatalf("junk: mode %d", w.mode)
	}
	pressKey(t, w, ctx, ebiten.KeyEnter)
	if w.mode != wlPreview {
		t.Error("a file with no words was imported")
	}
	if err := w.queueFile("empty.txt", nil); err == nil {
		t.Error("an empty file was queued")
	}
}

// Every screen fits 640x360 and draws.
func TestEditorScreensFit(t *testing.T) {
	w, ctx := wordListsScreen(t)
	f := ctx.Font
	dst := ebiten.NewImage(game.ScreenW, game.ScreenH)
	w.Draw(dst, ctx)

	// The Word Lists help lines.
	for _, s := range []string{
		"↑/↓ choose  N new list  E edit  Space mark  X delete  F Word Forge",
		"I import (or drop files)  A mark all  S save all  Esc back",
		"Drop .txt files on the page to import  A mark all  S save all  Esc back",
	} {
		if got := f.Width(s, 1); got > game.ScreenW-24 {
			t.Errorf("%q is %dpx", s, got)
		}
	}
	for _, h := range edHint {
		if got := f.Width(h.s, 1); got > edHintW-24 {
			t.Errorf("hint %q is %dpx, panel text is %dpx", h.s, got, edHintW-24)
		}
	}
	for _, s := range []string{
		"↑/↓ line  Enter next line  Tab accent  F3 next problem",
		"↑/↓ line  Enter next line  Tab accent  F2 Greek  F3 next problem",
		"Ctrl/Cmd+S save  Esc done.  Lists you type stay on this device.",
		"! 1 file has lines I can't read: W shows, E fixes",
		brokenMessage(12),
	} {
		if got := f.Width(s, 1); got > game.ScreenW-24 {
			t.Errorf("%q is %dpx", s, got)
		}
	}
	if got := edBodyY + edRows*edRowH; got > edY+edH {
		t.Errorf("the lines end at %d, the window at %d", got, edY+edH)
	}

	// The editor with many lines, long text, a problem, Greek and the dialogs.
	for _, code := range []string{"fr", "grc"} {
		w.openEditor(ctx, nil)
		w.ed.lang = langIndex(t, code)
		typeInEditor(w, ctx, strings.Repeat("A very long title ", 8))
		w.ed.Enter()
		w.ed.Enter()
		for i := 0; i < 30; i++ {
			typeInEditor(w, ctx, "word = "+strings.Repeat("answer ", 20))
			w.ed.Enter()
		}
		typeInEditor(w, ctx, "no equals here")
		for _, ask := range []int{edAskNone, edAskLeave, edAskSkip} {
			w.edAsk = ask
			w.Draw(dst, ctx)
		}
		w.ed.focus = edTitle
		w.Draw(dst, ctx)
		w.closeEdit()
	}

	// The skipped-lines window with more lines than fit.
	var b strings.Builder
	for i := 0; i < 40; i++ {
		b.WriteString("a line with no equals sign at all, which is long enough to be cut off at the edge of the window\n")
	}
	w.queueFile("many.txt", []byte(b.String()))
	w.Update(ctx)
	w.Draw(dst, ctx)
	if got := len(w.pvLines(ctx)); got < 80 {
		t.Errorf("only %d lines", got)
	}
	for i := 0; i < 100; i++ {
		pressKey(t, w, ctx, ebiten.KeyArrowDown)
	}
	w.Draw(dst, ctx)
	if last := len(w.pvLines(ctx)) - pvRowsShow; w.pvScroll != last {
		t.Errorf("scrolled to %d, last is %d", w.pvScroll, last)
	}
	for _, l := range w.pvLines(ctx) {
		if f.Width(l.s, 1) > pvW-40 {
			t.Errorf("%q is %dpx", l.s, f.Width(l.s, 1))
		}
	}
}
