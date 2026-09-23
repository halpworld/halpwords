package words

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"
	"sort"
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
	Entries  []Entry
	Source   string // file name, for error messages
}

// Parse reads a word list in the plain text format:
//
//	# comment
//	title: French - Animals
//	language: fr
//	## animals
//	dog = le chien
//	friend = l'ami | l'amie
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
	l := &List{Source: source}
	tag := ""
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
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("%s line %d: expected \"english = answer\"", source, n)
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "title":
			l.Title = clean(val)
		case "language":
			l.Language = strings.ToLower(clean(val))
			if lang, ok := Find(l.Language); ok {
				l.Language = lang.Code
			}
		default:
			return nil, fmt.Errorf("%s line %d: unknown setting %q (use title: or language:)", source, n, strings.TrimSpace(key))
		}
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
	if l.Title == "" {
		l.Title = strings.TrimSuffix(path.Base(source), path.Ext(source))
	}
	return l, nil
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
// grouped by tag, in the order each tag first appears.
func (l *List) Format() []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "title: %s\nlanguage: %s\n", l.Title, l.Language)
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
		b.WriteString("\n")
		if t != "" {
			fmt.Fprintf(&b, "## %s\n", t)
		}
		for _, e := range byTag[t] {
			fmt.Fprintf(&b, "%s = %s\n", e.Prompt, strings.Join(e.Answers, " | "))
		}
	}
	return []byte(b.String())
}

// Merge adds the words in o that l does not have yet, and any new answers
// for words it has, and returns how many words it added.
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
