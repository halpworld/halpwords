package scene

import (
	"strings"
	"testing"

	"github.com/halpworld/halpwords/pkg/words"
)

func typeLine(e *listEditor, s string) {
	for _, r := range s {
		e.Type(r)
	}
}

// langIndex returns the index of the language with code in words.Languages.
func langIndex(t *testing.T, code string) int {
	t.Helper()
	for i, l := range words.Languages {
		if l.Code == code {
			return i
		}
	}
	t.Fatalf("no language %q", code)
	return 0
}

func TestListEditorChecksAsYouType(t *testing.T) {
	e := newListEditor(langIndex(t, "fr"))
	e.focus = edBody
	typeLine(e, "dog = chien")
	e.Enter()
	typeLine(e, "the cat chat")
	e.Enter()
	typeLine(e, "bird = oiseau | l'oiseau")
	l, probs := e.Checked()
	if len(l.Entries) != 2 || len(probs) != 1 || probs[0].Line != 2 {
		t.Fatalf("entries %d, problems %v", len(l.Entries), probs)
	}
	if e.Problem(2) == "" || e.Problem(1) != "" || e.Problem(3) != "" {
		t.Errorf("per-line problems wrong: %q %q %q", e.Problem(1), e.Problem(2), e.Problem(3))
	}
	if !strings.Contains(e.summary(), "2 words, 1 line to fix (first: line 2)") {
		t.Errorf("summary = %q", e.summary())
	}
	e.cur = 2
	if !e.NextBad() || e.cur != 1 {
		t.Errorf("NextBad went to %d", e.cur)
	}
	// Fixing the line clears the problem.
	e.cur = 1
	for e.Backspace() && e.lines[1].Len() > 0 {
	}
	typeLine(e, "cat = chat")
	if e.Problem(2) != "" || len(e.probs) != 0 {
		t.Errorf("still a problem: %v", e.probs)
	}
}

func TestListEditorResult(t *testing.T) {
	e := newListEditor(langIndex(t, "fr"))
	e.focus = edBody
	typeLine(e, "dog = chien")
	if _, _, err := e.Result(nil); err == nil || e.focus != edTitle {
		t.Errorf("no title: err %v, focus %d", err, e.focus)
	}
	typeLine(e, "")
	e.focus = edTitle
	typeLine(e, "  Week   5 ")
	e.focus = edBody
	e.Enter()
	typeLine(e, "oops")
	l, skipped, err := e.Result(&words.List{File: "x.txt", ID: "abc", Version: 3})
	if err != nil {
		t.Fatal(err)
	}
	if l.Title != "Week 5" || l.Language != "fr" || l.File != "x.txt" || l.ID != "abc" || l.Version != 3 || len(l.Entries) != 1 || skipped != 1 {
		t.Errorf("got %+v, skipped %d", l, skipped)
	}
	// The result reads back with Parse.
	if _, err := words.Parse(strings.NewReader(words.Format(l)), "x.txt"); err != nil {
		t.Errorf("Parse: %v", err)
	}

	empty := newListEditor(0)
	empty.title = e.title
	if _, _, err := empty.Result(nil); err == nil {
		t.Error("a list with no words was accepted")
	}
}

func TestListEditorNoHeadersInLines(t *testing.T) {
	e := newListEditor(0)
	e.focus = edBody
	typeLine(e, "title: Sneaky")
	e.Enter()
	typeLine(e, "id: 123")
	e.Enter()
	typeLine(e, "level: A1")
	e.Enter()
	typeLine(e, "dog = chien")
	if e.Problem(1) == "" || e.Problem(2) == "" || e.Problem(3) != "" {
		t.Errorf("problems %q %q %q", e.Problem(1), e.Problem(2), e.Problem(3))
	}
	e.focus = edTitle
	typeLine(e, "Mine")
	l, _, err := e.Result(nil)
	if err != nil || l.Title != "Mine" || l.ID != "" || l.Level != "A1" {
		t.Errorf("got %+v, %v", l, err)
	}
}

func TestListEditorLines(t *testing.T) {
	e := newListEditor(0)
	e.focus = edBody
	typeLine(e, "a = b")
	e.Enter()
	e.Enter()
	typeLine(e, "c = d")
	if len(e.lines) != 3 || e.cur != 2 {
		t.Fatalf("lines %d cur %d", len(e.lines), e.cur)
	}
	// Enter in the middle opens a line below.
	e.cur = 0
	e.Enter()
	if len(e.lines) != 4 || e.cur != 1 || e.lines[1].Len() != 0 {
		t.Errorf("lines %d cur %d", len(e.lines), e.cur)
	}
	// Backspace on an empty line removes it.
	e.Backspace()
	if len(e.lines) != 3 || e.cur != 0 || e.text() != "a = b\n\nc = d" {
		t.Errorf("text %q cur %d", e.text(), e.cur)
	}
	// Up from the first line goes to the language, then the title.
	if !e.Up() || e.focus != edLang {
		t.Errorf("focus %d", e.focus)
	}
	if !e.Up() || e.focus != edTitle {
		t.Errorf("focus %d", e.focus)
	}
	if e.Up() {
		t.Error("moved above the title")
	}
	e.Down()
	e.Down()
	if e.focus != edBody {
		t.Errorf("focus %d", e.focus)
	}
	// The title and language don't take lines.
	e.focus = edLang
	if e.Type('x') {
		t.Error("typed into the language")
	}
	e.Language(-1)
	if e.lang != len(words.Languages)-1 {
		t.Errorf("lang %d", e.lang)
	}
}

func TestListEditorLongLine(t *testing.T) {
	e := newListEditor(0)
	e.focus = edBody
	typeLine(e, "word = "+strings.Repeat("a", 100)) // over the 60-letter answer limit, under the line's
	if e.lines[0].Len() != 107 {
		t.Fatalf("line has %d letters", e.lines[0].Len())
	}
	if e.Problem(1) == "" || !strings.Contains(e.Problem(1), "too long") {
		t.Errorf("problem = %q", e.Problem(1))
	}
	typeLine(e, strings.Repeat("b", 200))
	if e.lines[0].Len() != edLineMax {
		t.Errorf("line has %d letters, limit %d", e.lines[0].Len(), edLineMax)
	}
}

func TestListEditorAccents(t *testing.T) {
	// French: Tab cycles the accent. Irish: fadas.
	for code, want := range map[string]string{"fr": "tea = thé", "ga": "tea = tá"} {
		e := newListEditor(langIndex(t, code))
		e.focus = edBody
		typeLine(e, "tea = th")
		if code == "ga" {
			e.Backspace()
			e.Backspace()
			typeLine(e, "t")
			typeLine(e, "a")
		} else {
			typeLine(e, "e")
		}
		if !e.Accent() {
			t.Fatalf("%s: no accent", code)
		}
		if got := e.lines[0].Text(); got != want {
			// the first accent in the cycle may be a grave: go on to the acute
			for i := 0; i < 4 && e.lines[0].Text() != want; i++ {
				e.Accent()
			}
			if got = e.lines[0].Text(); got != want {
				t.Errorf("%s: %q, want %q", code, got, want)
			}
		}
	}
	// Typed accents arrive as typed, in one letter each, composed.
	e := newListEditor(langIndex(t, "ga"))
	e.focus = edBody
	typeLine(e, "tea = ta\u0301 \u00f3")
	if got := e.lines[0].Text(); got != "tea = t\u00e1 \u00f3" {
		t.Errorf("got %q", got)
	}
}

func TestListEditorGreek(t *testing.T) {
	e := newListEditor(langIndex(t, "grc"))
	e.focus = edBody
	// English before the =, Greek keys after it. "=" and "|" after a space are
	// written as they are; "w=" is ῶ (circumflex).
	typeLine(e, "water = u(dwr | w=")
	if got := e.lines[0].Text(); got != "water = \u1f51\u03b4\u03c9\u03c1 | \u1ff6" {
		t.Errorf("got %q", got)
	}
	l, probs := e.Checked()
	if len(probs) != 0 || len(l.Entries) != 1 || l.Entries[0].Prompt != "water" || len(l.Entries[0].Answers) != 2 {
		t.Errorf("entries %+v, problems %v", l.Entries, probs)
	}
	// F2 flips the Greek keys: now the words before the = are Greek and the
	// answers are Latin.
	e.greekSwap = true
	e.Enter()
	typeLine(e, "sea = abc")
	if got := e.lines[1].Text(); got != "\u03c3\u03b5\u03b1 = abc" {
		t.Errorf("flipped: %q", got)
	}
	// A gap-fill sentence is Greek from the start.
	e.greekSwap = false
	e.Enter()
	typeLine(e, ">> a ___ | a")
	if got := e.lines[2].Text(); !strings.Contains(got, "\u03b1") {
		t.Errorf("gap line: %q", got)
	}
}

func TestOpenListEditor(t *testing.T) {
	l := &words.List{Title: "Pets", Language: "fr", ID: "id1", Version: 2, Level: "A1", File: "pets.txt",
		Entries: []words.Entry{{Prompt: "dog", Answers: []string{"chien"}}, {Prompt: "cat", Answers: []string{"chat", "minou"}, Tag: "pets"}}}
	e := openListEditor(l, "")
	if e.title.Text() != "Pets" || e.language().Code != "fr" || e.changed() {
		t.Fatalf("title %q lang %s changed %v", e.title.Text(), e.language().Code, e.changed())
	}
	for _, h := range []string{"title:", "language:", "id:", "version:"} {
		if strings.Contains(e.text(), h) {
			t.Errorf("body has %q:\n%s", h, e.text())
		}
	}
	back, skipped, err := e.Result(l)
	if err != nil || skipped != 0 {
		t.Fatal(err, skipped)
	}
	if words.Format(back) != words.Format(l) {
		t.Errorf("edit without changes changed the list:\n%s\nvs\n%s", words.Format(back), words.Format(l))
	}
	typeLine(e, "x")
	if !e.changed() {
		t.Error("typing is not a change")
	}

	// A file the game can't read opens as it is, problems marked.
	raw := "\uFEFFtitle: Mine\nlanguage: fr\r\ndog = chien\r\nbad line\r\n"
	e = openListEditor(&words.List{Title: "Mine", Language: "fr", File: "mine.txt"}, raw)
	if e.Problem(2) == "" || e.Problem(1) != "" {
		t.Errorf("problems %v\n%s", e.probs, e.text())
	}
}
