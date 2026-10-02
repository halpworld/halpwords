package scene

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/halpworld/halpwords/internal/typing"
	"github.com/halpworld/halpwords/pkg/words"
)

// The quick list editor's limits.
const (
	edTitleMax = 60
	edLineMax  = 200 // a prompt and several answers
	edLinesMax = 500
)

// Where the typing goes in the quick list editor.
const (
	edTitle = iota
	edLang
	edBody
)

// listEditor is the model of the quick list editor: a title, a language
// and a body of lines in the word list format. The lines are checked as
// they are typed, with words.ParseLenient. The text is never sent anywhere.
type listEditor struct {
	focus int
	title *typing.Field
	lang  int // index into words.Languages
	lines []*typing.Field
	cur   int // the line being typed, when focus is edBody

	// greekSwap flips the Greek keys (F2) from what the line calls for.
	greekSwap bool

	start string // the text when the editor opened, to tell if it changed

	list  *words.List // the good lines, when checked
	probs []words.Problem
	byLn  map[int]string
	ok    bool // list and probs are up to date
}

// newListEditor opens an empty editor for the language at index lang.
func newListEditor(lang int) *listEditor {
	e := &listEditor{lang: lang, focus: edTitle}
	e.title = &typing.Field{Lang: words.Languages[lang], Limit: edTitleMax}
	e.lines = []*typing.Field{e.newLine("")}
	e.start = e.snapshot()
	return e
}

// openListEditor opens the editor on an existing list. raw is the text of
// the list's file when the file could not be read as a list; otherwise the
// words are written out from l.
func openListEditor(l *words.List, raw string) *listEditor {
	lang := 0
	for i, x := range words.Languages {
		if x.Code == l.Language {
			lang = i
		}
	}
	e := newListEditor(lang)
	e.setField(e.title, l.Title)
	e.focus = edBody
	body := raw
	if body == "" {
		body = words.Format(l)
	}
	e.lines = nil
	for _, ln := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if ownHeader(ln) {
			continue
		}
		e.lines = append(e.lines, e.newLine(ln))
	}
	// Format's blank line after the headers is not a word.
	for len(e.lines) > 1 && e.lines[0].Len() == 0 {
		e.lines = e.lines[1:]
	}
	for len(e.lines) > 1 && e.lines[len(e.lines)-1].Len() == 0 {
		e.lines = e.lines[:len(e.lines)-1]
	}
	if len(e.lines) == 0 {
		e.lines = []*typing.Field{e.newLine("")}
	}
	e.cur = len(e.lines) - 1
	e.start = e.snapshot()
	return e
}

// ownHeader reports whether line sets what the editor's own boxes and the
// website own: the title, the language, the id and the version.
func ownHeader(line string) bool {
	key, _, ok := strings.Cut(strings.TrimPrefix(strings.TrimSpace(line), "\uFEFF"), ":")
	if !ok || strings.ContainsAny(key, "=|") {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "title", "language", "id", "version":
		return true
	}
	return false
}

func (e *listEditor) newLine(text string) *typing.Field {
	f := &typing.Field{Lang: words.Languages[e.lang], Limit: edLineMax}
	e.setField(f, text)
	return f
}

// setField puts text in f as typed, without the Greek keys.
func (e *listEditor) setField(f *typing.Field, text string) {
	f.Reset()
	g, lim := f.Greek, f.Limit
	f.Greek, f.Limit = false, 1<<20
	for _, r := range text {
		f.Type(r)
	}
	f.Greek, f.Limit = g, lim
	f.UsedBackspace = false
}

func (e *listEditor) language() *words.Language { return words.Languages[e.lang] }

// field is the one being typed in.
func (e *listEditor) field() *typing.Field {
	switch e.focus {
	case edTitle:
		return e.title
	case edBody:
		return e.lines[e.cur]
	}
	return nil
}

// text is the body, one line per line.
func (e *listEditor) text() string {
	ls := make([]string, len(e.lines))
	for i, f := range e.lines {
		ls[i] = f.Text()
	}
	return strings.Join(ls, "\n")
}

// changed reports whether anything was typed since the editor opened.
func (e *listEditor) changed() bool {
	return e.snapshot() != e.start
}

func (e *listEditor) snapshot() string {
	return e.title.Text() + "\n" + e.text() + "\n" + e.language().Code
}

// greekOn reports whether Latin keys type Greek on f, in a Greek list: the
// answers, not the English words, so a line has Greek after its "=" (or
// when it is a gap-fill sentence). F2 flips it.
func (e *listEditor) greekOn(f *typing.Field) bool {
	if f == e.title || e.language().Script != words.ScriptGreek {
		return false
	}
	t := strings.TrimSpace(f.Text())
	return (strings.Contains(t, "=") || strings.HasPrefix(t, ">>")) != e.greekSwap
}

// Type types r into the title or the line, and reports whether it was
// taken. "=" and "|" are marks in the Greek keys, so after a space (as in
// "dog = chien") they are written as they are.
func (e *listEditor) Type(r rune) bool {
	f := e.field()
	if f == nil {
		return false
	}
	f.Lang = e.language()
	f.Greek = e.greekOn(f)
	if f.Greek && (r == '=' || r == '|') {
		t := []rune(f.Text())
		if len(t) == 0 || unicode.IsSpace(t[len(t)-1]) {
			f.Greek = false
		}
	}
	ok := f.Type(r)
	if ok {
		e.ok = false
	}
	return ok
}

// Backspace deletes the last character, or joins an empty line to the one
// before it.
func (e *listEditor) Backspace() bool {
	f := e.field()
	if f == nil {
		return false
	}
	if f.Len() == 0 && e.focus == edBody && len(e.lines) > 1 {
		e.lines = append(e.lines[:e.cur], e.lines[e.cur+1:]...)
		e.cur = max(0, e.cur-1)
		e.ok = false
		return true
	}
	if f.Backspace() {
		e.ok = false
		return true
	}
	return false
}

// Accent changes the last letter to its next accented form (Tab).
func (e *listEditor) Accent() bool {
	f := e.field()
	if f == nil {
		return false
	}
	f.Lang = e.language()
	f.Greek = e.greekOn(f)
	if f.CycleAccent() {
		e.ok = false
		return true
	}
	return false
}

// Enter moves on: from the title to the language to the lines, where it
// starts a new line below the one being typed.
func (e *listEditor) Enter() bool {
	switch e.focus {
	case edTitle, edLang:
		e.focus++
		return true
	}
	if len(e.lines) >= edLinesMax {
		return false
	}
	e.lines = append(e.lines[:e.cur+1], append([]*typing.Field{e.newLine("")}, e.lines[e.cur+1:]...)...)
	e.cur++
	e.ok = false
	return true
}

// Up and Down move between the title, the language and the lines.
func (e *listEditor) Up() bool {
	switch {
	case e.focus == edBody && e.cur > 0:
		e.cur--
	case e.focus > edTitle:
		if e.focus == edBody {
			e.focus = edLang
		} else {
			e.focus = edTitle
		}
	default:
		return false
	}
	return true
}

func (e *listEditor) Down() bool {
	switch {
	case e.focus < edBody:
		e.focus++
	case e.cur < len(e.lines)-1:
		e.cur++
	default:
		return false
	}
	return true
}

// Language changes the language by step (±1).
func (e *listEditor) Language(step int) {
	n := len(words.Languages)
	e.lang = (e.lang + step + n) % n
	e.ok = false
}

// bodyHeader reports why a line that sets the title, language, id or
// version is not allowed in the lines.
const bodyHeader = "the title and language have their own boxes above; id and version belong to the website"

// check parses the lines, if they changed since the last check.
func (e *listEditor) check() {
	if e.ok {
		return
	}
	l, probs := words.ParseLenient(strings.NewReader(e.text()), "list.txt")
	for i, f := range e.lines {
		if ownHeader(f.Text()) {
			probs = append(probs, words.Problem{Line: i + 1, Text: f.Text(), Reason: bodyHeader})
		}
	}
	e.byLn = map[int]string{}
	for _, p := range probs {
		if _, dup := e.byLn[p.Line]; !dup {
			e.byLn[p.Line] = p.Reason
		}
	}
	e.list, e.probs, e.ok = l, probs, true
}

// Checked returns the words typed so far and the lines that are not words.
func (e *listEditor) Checked() (*words.List, []words.Problem) {
	e.check()
	return e.list, e.probs
}

// Problem returns why line n (from 1) is not a word, or "".
func (e *listEditor) Problem(n int) string {
	e.check()
	return e.byLn[n]
}

// Result makes the list the editor describes, with only its good lines.
// orig is the list being edited, or nil for a new one: its id and version
// are kept. skipped is how many lines were left out.
func (e *listEditor) Result(orig *words.List) (l *words.List, skipped int, err error) {
	e.check()
	title := strings.Join(strings.Fields(e.title.Text()), " ")
	if title == "" {
		e.focus = edTitle
		return nil, 0, errors.New("give the list a title first")
	}
	l = &words.List{Title: title, Language: e.language().Code, Entries: e.list.Entries, Cloze: e.list.Cloze,
		Level: e.list.Level, Source: e.list.Source, Licence: e.list.Licence}
	if len(l.Entries) == 0 {
		return nil, 0, errors.New("type at least one word, like: dog = chien")
	}
	if orig != nil {
		l.File, l.ID, l.Version = orig.File, orig.ID, orig.Version
	}
	return l, len(e.probs), nil
}

// summary says how the lines are doing.
func (e *listEditor) summary() string {
	l, probs := e.Checked()
	s := plural(len(l.Entries), "word")
	switch len(probs) {
	case 0:
	case 1:
		s += ", 1 line to fix"
	default:
		s += fmt.Sprintf(", %d lines to fix", len(probs))
	}
	if first := e.firstBad(); first > 0 {
		s += fmt.Sprintf(" (first: line %d)", first)
	}
	return s
}

// firstBad returns the number of the first line that is not a word, or 0.
func (e *listEditor) firstBad() int {
	_, probs := e.Checked()
	for _, p := range probs {
		if p.Line > 0 {
			return p.Line
		}
	}
	return 0
}

// NextBad moves to the next line after the current one that is not a word,
// going round to the first, and reports whether there was one.
func (e *listEditor) NextBad() bool {
	e.check()
	n := len(e.lines)
	for i := 1; i <= n; i++ {
		j := (e.cur + i) % n
		if e.byLn[j+1] != "" {
			e.focus, e.cur = edBody, j
			return true
		}
	}
	return false
}
