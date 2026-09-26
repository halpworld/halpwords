package puzzle

import (
	"math/rand/v2"
	"strings"

	"github.com/halpworld/halpwords/pkg/words"
)

// Generated is content for puzzles beyond the word lists' own words: what
// an AI wrote, and the lists' own gap-fill sentences (see FromLists).
// Everything in it has been checked, and the local grader still marks
// every answer.
type Generated struct {
	// Riddles are extra English riddles, by English word in lower case.
	Riddles map[string][]string
	// Cloze are sentences in the language being learned with a gap for a
	// word, by words.Key.
	Cloze map[string][]ClozeLine
}

// ClozeLine is a sentence with a gap (ClozeGap) where a word goes, and
// the whole sentence in English, which may be empty. Answer, when set, is
// the only answer that fits the gap; otherwise any of the word's answers
// does.
type ClozeLine struct {
	Text, English string
	Answer        string
}

// ClozeGap marks the missing word in a ClozeLine.
const ClozeGap = words.ClozeGap

// FromLists returns the gap-fill sentences written in lists (their ">>"
// lines) as Generated content, by the words they are for, or nil if the
// lists have none. It works without an AI.
func FromLists(lists []*words.List) *Generated {
	var g *Generated
	for _, l := range lists {
		for _, c := range l.Cloze {
			for _, e := range l.Entries {
				a, ok := c.Match(e)
				if !ok {
					continue
				}
				if g == nil {
					g = &Generated{Cloze: map[string][]ClozeLine{}}
				}
				k := words.Key(e)
				g.Cloze[k] = append(g.Cloze[k], ClozeLine{Text: c.Sentence, Answer: a})
			}
		}
	}
	return g
}

// Add adds o's riddles and sentences to g and returns g. A nil g or o is
// empty; g is made if it is nil and o is not.
func (g *Generated) Add(o *Generated) *Generated {
	if o == nil {
		return g
	}
	if g == nil {
		g = &Generated{}
	}
	for k, v := range o.Riddles {
		if g.Riddles == nil {
			g.Riddles = map[string][]string{}
		}
		g.Riddles[k] = append(g.Riddles[k], v...)
	}
	for k, v := range o.Cloze {
		if g.Cloze == nil {
			g.Cloze = map[string][]ClozeLine{}
		}
		g.Cloze[k] = append(g.Cloze[k], v...)
	}
	return g
}

// newCloze asks for the word missing from a sentence. It returns nil if no
// word in the deck has a sentence.
func newCloze(deck dealer, lang *words.Language, gen *Generated, rng *rand.Rand) *typed {
	if gen == nil || len(gen.Cloze) == 0 {
		return nil
	}
	e, id, ok := deck.NextWhere(func(e words.Entry) bool { return len(gen.Cloze[words.Key(e)]) > 0 })
	if !ok {
		return nil
	}
	lines := gen.Cloze[words.Key(e)]
	l := lines[rng.IntN(len(lines))]
	grade, answer := e, e.Answers[0]
	if l.Answer != "" {
		grade, answer = only(e, l.Answer), l.Answer
	}
	extra := []string{strings.Replace(l.Text, ClozeGap, answer, 1)}
	if l.English != "" {
		extra = append(extra, "("+l.English+")")
	}
	return &typed{
		kind: Cloze, answer: Foreign, id: id,
		ask:   "Fill the gap in " + lang.Name + ":",
		clue:  strings.Replace(l.Text, ClozeGap, "_____", 1) + "  (" + e.Prompt + ")",
		grade: grade, lang: lang, rules: lang.Defaults, shown: e.Prompt,
		extra: extra,
	}
}
