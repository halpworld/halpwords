package words

import (
	"bufio"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"
	"unicode/utf8"
)

// Problem is a line of a word list that ParseLenient skipped.
type Problem struct {
	Line   int    // line number, from 1; 0 when it is not about one line
	Text   string // the line, trimmed
	Reason string // a short plain sentence for a parent or a child
}

// String returns the problem as "line 4: reason".
func (p Problem) String() string {
	if p.Line == 0 {
		return p.Reason
	}
	return fmt.Sprintf("line %d: %s", p.Line, p.Reason)
}

// ParseLenient reads a word list as ParseImport does (the language may be
// left out, and a word can be written "english<Tab>answer"), but instead of
// stopping at the first bad line it skips each one and describes it in the
// problems it returns, in line order. The list has every good line. It is
// never nil; it may have no words, and Language is "" when the list gives
// none or one the game does not know (which is a problem too).
//
// A line is skipped when it has an unknown setting, no "=", nothing before
// or after the "=", an answer longer than MaxAnswerRunes, or an English word
// the list already has; and a gap-fill line is skipped when its sentence
// has no single gap or its answer is not one of the list's answers.
func ParseLenient(r io.Reader, source string) (*List, []Problem) {
	l := &List{File: source}
	var probs []Problem
	skip := func(n int, text, reason string) {
		probs = append(probs, Problem{Line: n, Text: text, Reason: reason})
	}
	type gap struct {
		c    ClozeLine
		n    int
		text string
	}
	var gaps []gap
	seen := map[string]int{} // prompt, folded, to the line it was on
	langLine, langText := 0, ""
	tag := ""
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 1<<20)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\uFEFF"))
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "##"):
			tag = clean(strings.TrimLeft(line, "#"))
			continue
		case strings.HasPrefix(line, "#"):
			continue
		case strings.HasPrefix(line, ">>"):
			c, err := parseCloze(strings.TrimPrefix(line, ">>"), tag)
			if err != nil {
				skip(n, line, err.Error())
				continue
			}
			gaps = append(gaps, gap{c, n, line})
			continue
		}
		if key, val, ok := strings.Cut(line, ":"); ok {
			key = strings.ToLower(strings.TrimSpace(key))
			if strings.HasPrefix(key, "x-") && !strings.ContainsAny(key, " =") {
				continue
			}
			if headers[key] && !strings.Contains(key, "=") {
				if err := l.setHeader(key, val); err != nil {
					skip(n, line, err.Error())
				}
				if key == "language" {
					langLine, langText = n, line
				}
				continue
			}
		}
		prompt, answers, ok := strings.Cut(line, "=")
		if !ok {
			prompt, answers, ok = strings.Cut(line, "\t")
		}
		if !ok {
			if key, _, colon := strings.Cut(line, ":"); colon && !strings.ContainsAny(key, " ") {
				skip(n, line, fmt.Sprintf("%q is not a setting. Words are written english = answer (settings are title:, language:, level:, source: and licence:)", key))
				continue
			}
			skip(n, line, "no '=' here. Write it as: english = answer")
			continue
		}
		e := Entry{Prompt: clean(prompt), Tag: tag}
		for _, a := range strings.Split(answers, "|") {
			if a = clean(a); a != "" {
				e.Answers = append(e.Answers, a)
			}
		}
		switch {
		case e.Prompt == "":
			skip(n, line, "nothing before the '='. Write it as: english = answer")
			continue
		case len(e.Answers) == 0:
			skip(n, line, "nothing after the '='. Write it as: english = answer")
			continue
		case slices.ContainsFunc(e.Answers, func(a string) bool { return utf8.RuneCountInString(a) > MaxAnswerRunes }):
			skip(n, line, fmt.Sprintf("an answer is longer than %d letters, which is too long to type", MaxAnswerRunes))
			continue
		}
		key := strings.ToLower(e.Prompt)
		if first, dup := seen[key]; dup {
			skip(n, line, fmt.Sprintf("%q is already on line %d. Put other answers on that line after a |", e.Prompt, first))
			continue
		}
		seen[key] = n
		l.Entries = append(l.Entries, e)
	}
	if err := sc.Err(); err != nil {
		skip(n+1, "", "could not read the rest of the file: "+err.Error())
	}
	if l.Language != "" {
		if _, ok := Lookup(l.Language); !ok {
			skip(langLine, langText, fmt.Sprintf("unknown language %q (use fr, la, grc or ga)", l.Language))
			l.Language = ""
		}
	}
	for _, g := range gaps {
		if !slices.ContainsFunc(l.Entries, g.c.Fits) {
			skip(g.n, g.text, fmt.Sprintf("the gap's answer %q is not one of the list's answers", g.c.Answer))
			continue
		}
		l.Cloze = append(l.Cloze, g.c)
	}
	if l.Title == "" {
		l.Title = strings.TrimSuffix(path.Base(source), path.Ext(source))
	}
	slices.SortStableFunc(probs, func(a, b Problem) int { return a.Line - b.Line })
	return l, probs
}
