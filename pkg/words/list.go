package words

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Entry is one word to learn.
type Entry struct {
	Prompt  string   // English
	Answers []string // accepted answers; the first is shown as the solution
	Tag     string   // from the "## tag" group it is in
}

// List is a word list file.
type List struct {
	Title    string
	Language string // language code, e.g. "fr"
	ID       string // "id:", the server's list ID; optional
	Version  int    // "version:", set by the server; 0 when there is none
	Level    string // "level:", such as a CEFR level ("A1"); optional
	Source   string // "source:", where the words come from; optional
	Licence  string // "licence:", such as "CC-BY-4.0"; optional
	Entries  []Entry
	Cloze    []ClozeLine // ">>" gap-fill sentences
	File     string      // file name, for error messages
}

// ClozeLine is a gap-fill sentence from a list's ">>" line: a sentence in
// the language being learned with a gap (ClozeGap) where Answer goes.
// Answer is one of the list's answers.
type ClozeLine struct {
	Sentence string
	Answer   string
	Tag      string // from the "## tag" group it is in
}

// ClozeGap marks the gap in a ClozeLine's sentence.
const ClozeGap = "___"

// Fits reports whether c's answer is one of e's answers, ignoring case,
// spacing, apostrophe styles and Unicode composition.
func (c ClozeLine) Fits(e Entry) bool {
	_, ok := c.Match(e)
	return ok
}

// Match returns the answer of e that c's answer is, as e spells it, and
// whether there is one (see Fits).
func (c ClozeLine) Match(e Entry) (string, bool) {
	want := normalize(c.Answer, Rules{})
	for _, a := range e.Answers {
		if normalize(a, Rules{}) == want {
			return a, true
		}
	}
	return "", false
}

// headers are the "key: value" lines a list can start with. Keys starting
// with "x-" are ignored, so newer lists can add settings that older games
// skip.
var headers = map[string]bool{
	"title": true, "language": true, "id": true, "version": true,
	"level": true, "source": true, "licence": true,
}

// Parse reads a word list in the plain text format:
//
//	# comment
//	title: French - Animals
//	language: fr
//	id: 01J9Z6...            (optional: id, version, level, source, licence)
//	x-anything: ignored
//	## animals
//	dog = le chien
//	friend = l'ami | l'amie
//	>> Le ___ aboie. | chien
//
// A ">>" line is a gap-fill sentence: the sentence with ___ for the gap,
// then after the last "|" the answer, which must be one of the list's
// answers.
func Parse(r io.Reader, source string) (*List, error) {
	l, err := parse(r, source, false)
	if err != nil {
		return nil, err
	}
	if l.Language == "" {
		return nil, fmt.Errorf("%s: missing \"language:\" line", source)
	}
	return l, nil
}

// ParseImport reads a word list being imported. It is more forgiving than
// Parse: the language may be left out (Language is then ""), and a word can
// also be written "english<Tab>answer", as spreadsheets and flashcard sites
// export them.
func ParseImport(r io.Reader, source string) (*List, error) {
	return parse(r, source, true)
}

func parse(r io.Reader, source string, loose bool) (*List, error) {
	l := &List{File: source}
	tag := ""
	clozeAt := map[int]int{} // line number of each Cloze line, for errors
	sc := bufio.NewScanner(r)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\uFEFF"))
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "##"):
			tag = strings.TrimSpace(strings.TrimLeft(line, "#"))
			continue
		case strings.HasPrefix(line, "#"):
			continue
		case strings.HasPrefix(line, ">>"):
			c, err := parseCloze(strings.TrimPrefix(line, ">>"), tag)
			if err != nil {
				return nil, fmt.Errorf("%s line %d: %v", source, n, err)
			}
			clozeAt[len(l.Cloze)] = n
			l.Cloze = append(l.Cloze, c)
			continue
		}
		if key, val, ok := strings.Cut(line, ":"); ok {
			key = strings.ToLower(strings.TrimSpace(key))
			if strings.HasPrefix(key, "x-") && !strings.ContainsAny(key, " =") {
				continue
			}
			if headers[key] {
				if err := l.setHeader(key, val); err != nil {
					return nil, fmt.Errorf("%s line %d: %v", source, n, err)
				}
				continue
			}
		}
		prompt, answers, ok := strings.Cut(line, "=")
		if !ok && loose {
			prompt, answers, ok = strings.Cut(line, "\t")
		}
		if ok {
			e := Entry{Prompt: clean(prompt), Tag: tag}
			for _, a := range strings.Split(answers, "|") {
				if a = clean(a); a != "" {
					e.Answers = append(e.Answers, a)
				}
			}
			if e.Prompt == "" || len(e.Answers) == 0 {
				return nil, fmt.Errorf("%s line %d: expected \"english = answer\"", source, n)
			}
			l.Entries = append(l.Entries, e)
			continue
		}
		key, _, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("%s line %d: expected \"english = answer\"", source, n)
		}
		return nil, fmt.Errorf("%s line %d: unknown setting %q (use title:, language:, id:, version:, level:, source: or licence:)", source, n, strings.TrimSpace(key))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if _, ok := Lookup(l.Language); !ok && l.Language != "" {
		return nil, fmt.Errorf("%s: unknown language %q", source, l.Language)
	}
	if len(l.Entries) == 0 {
		return nil, fmt.Errorf("%s: no words", source)
	}
	for i, c := range l.Cloze {
		if !slices.ContainsFunc(l.Entries, c.Fits) {
			return nil, fmt.Errorf("%s line %d: the gap's answer %q is not one of the list's answers", source, clozeAt[i], c.Answer)
		}
	}
	if l.Title == "" {
		l.Title = strings.TrimSuffix(path.Base(source), path.Ext(source))
	}
	return l, nil
}

// setHeader sets the header key (in lower case) to val.
func (l *List) setHeader(key, val string) error {
	val = clean(val)
	switch key {
	case "title":
		l.Title = val
	case "language":
		l.Language = strings.ToLower(val)
		if lang, ok := Find(l.Language); ok {
			l.Language = lang.Code
		}
	case "id":
		l.ID = val
	case "version":
		v, err := strconv.Atoi(val)
		if err != nil || v < 0 {
			return fmt.Errorf("version %q is not a whole number", val)
		}
		l.Version = v
	case "level":
		l.Level = val
	case "source":
		l.Source = val
	case "licence":
		l.Licence = val
	}
	return nil
}

// parseCloze reads a ">>" line after the ">>": "sentence with ___ | answer".
func parseCloze(s, tag string) (ClozeLine, error) {
	i := strings.LastIndex(s, "|")
	if i < 0 {
		return ClozeLine{}, fmt.Errorf("expected \">> sentence with %s | answer\"", ClozeGap)
	}
	c := ClozeLine{Sentence: clean(s[:i]), Answer: clean(s[i+1:]), Tag: tag}
	switch {
	case c.Answer == "" || c.Sentence == "":
		return c, fmt.Errorf("expected \">> sentence with %s | answer\"", ClozeGap)
	case strings.Count(c.Sentence, ClozeGap) != 1 || strings.Contains(c.Sentence, ClozeGap+"_"):
		return c, fmt.Errorf("a gap-fill sentence needs one gap, written %s", ClozeGap)
	}
	return c, nil
}

// LoadFS parses every .txt file in the root of fsys, sorted by file name.
func LoadFS(fsys fs.FS, dir string) ([]*List, error) {
	ents, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	sort.Slice(ents, func(i, j int) bool { return ents[i].Name() < ents[j].Name() })
	var lists []*List
	for _, e := range ents {
		if e.IsDir() || !strings.EqualFold(path.Ext(e.Name()), ".txt") {
			continue
		}
		f, err := fsys.Open(path.Join(dir, e.Name()))
		if err != nil {
			return lists, err
		}
		l, err := Parse(f, e.Name())
		f.Close()
		if err != nil {
			return lists, err
		}
		lists = append(lists, l)
	}
	return lists, nil
}

func clean(s string) string {
	return norm.NFC.String(strings.Join(strings.Fields(s), " "))
}

// Format writes l in the plain text format that Parse reads. Words are
// grouped by tag, in the order each tag first appears. Use the Format
// function to keep the words in their order.
func (l *List) Format() []byte {
	g := *l
	g.Entries = nil
	var tags []string
	byTag := map[string][]Entry{}
	for _, e := range l.Entries {
		if _, ok := byTag[e.Tag]; !ok {
			tags = append(tags, e.Tag)
		}
		byTag[e.Tag] = append(byTag[e.Tag], e)
	}
	// Untagged words must come before the first "##" line.
	sort.SliceStable(tags, func(i, j int) bool { return tags[i] == "" && tags[j] != "" })
	for _, t := range tags {
		g.Entries = append(g.Entries, byTag[t]...)
	}
	return []byte(Format(&g))
}

// Format writes l in the plain text format, so that Parse reads it back
// as the same List (apart from File). Words and gap-fill sentences keep
// their order and groups; the sentences come after the words. It doesn't
// check l: a prompt with "=" or an answer with "|" in it can't be written,
// so parse the result to check a list made in code.
func Format(l *List) string {
	var b strings.Builder
	fmt.Fprintf(&b, "title: %s\n", l.Title)
	if l.Language != "" {
		fmt.Fprintf(&b, "language: %s\n", l.Language)
	}
	version := ""
	if l.Version != 0 {
		version = strconv.Itoa(l.Version)
	}
	for _, h := range [...]struct{ key, val string }{
		{"id", l.ID}, {"version", version}, {"level", l.Level},
		{"source", l.Source}, {"licence", l.Licence},
	} {
		if h.val != "" {
			fmt.Fprintf(&b, "%s: %s\n", h.key, h.val)
		}
	}
	tag := ""
	group := func(t string) {
		if t != tag {
			b.WriteString("\n")
			fmt.Fprintf(&b, "%s\n", strings.TrimSpace("## "+t))
			tag = t
		}
	}
	b.WriteString("\n")
	for _, e := range l.Entries {
		group(e.Tag)
		fmt.Fprintf(&b, "%s = %s\n", e.Prompt, strings.Join(e.Answers, " | "))
	}
	for _, c := range l.Cloze {
		group(c.Tag)
		fmt.Fprintf(&b, ">> %s | %s\n", c.Sentence, c.Answer)
	}
	return b.String()
}

// Merge adds the words in o that l does not have yet, any new answers for
// words it has, and o's gap-fill sentences that l does not have, and
// returns how many words it added.
func (l *List) Merge(o *List) (added int) {
	for _, e := range o.Entries {
		i := slices.IndexFunc(l.Entries, func(x Entry) bool { return strings.EqualFold(x.Prompt, e.Prompt) })
		if i < 0 {
			e.Answers = slices.Clone(e.Answers)
			l.Entries = append(l.Entries, e)
			added++
			continue
		}
		for _, a := range e.Answers {
			if !slices.ContainsFunc(l.Entries[i].Answers, func(x string) bool { return strings.EqualFold(x, a) }) {
				l.Entries[i].Answers = append(l.Entries[i].Answers, a)
			}
		}
	}
	for _, c := range o.Cloze {
		if !slices.ContainsFunc(l.Cloze, func(x ClozeLine) bool { return x.Sentence == c.Sentence && x.Answer == c.Answer }) {
			l.Cloze = append(l.Cloze, c)
		}
	}
	return added
}

// FileName suggests a file name for a list called title, such as
// "french-animals.txt".
func FileName(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range norm.NFD.String(strings.ToLower(title)) {
		switch {
		case r < 0x80 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
		case unicode.Is(unicode.Mn, r):
			// accents: é becomes e
		default:
			dash = true
		}
	}
	if b.Len() == 0 {
		return "words.txt"
	}
	return b.String() + ".txt"
}
