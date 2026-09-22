package words

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"

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
		if prompt, answers, ok := strings.Cut(line, "="); ok {
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
		default:
			return nil, fmt.Errorf("%s line %d: unknown setting %q (use title: or language:)", source, n, strings.TrimSpace(key))
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if l.Language == "" {
		return nil, fmt.Errorf("%s: missing \"language:\" line", source)
	}
	if _, ok := Lookup(l.Language); !ok {
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
