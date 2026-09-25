package puzzle

import (
	"math/rand/v2"
	"strings"

	"github.com/halpworld/halpwords/internal/words"
)

// Generated is content an AI wrote for the word lists. Everything in it has
// been checked, and the local grader still marks every answer.
type Generated struct {
	// Riddles are extra English riddles, by English word in lower case.
	Riddles map[string][]string
	// Cloze are sentences in the language being learned with a gap for a
	// word, by words.Key.
	Cloze map[string][]ClozeLine
}

// ClozeLine is a sentence with a gap (ClozeGap) where a word goes, and
// the whole sentence in English.
type ClozeLine struct {
	Text, English string
}

// ClozeGap marks the missing word in a ClozeLine.
const ClozeGap = "___"

// newCloze asks for the word missing from a sentence. It returns nil if no
// word in the deck has a sentence.
func newCloze(deck *words.Deck, lang *words.Language, gen *Generated, rng *rand.Rand) *typed {
	if gen == nil || len(gen.Cloze) == 0 {
		return nil
	}
	e, id, ok := deck.NextWhere(func(e words.Entry) bool { return len(gen.Cloze[words.Key(e)]) > 0 })
	if !ok {
		return nil
	}
	lines := gen.Cloze[words.Key(e)]
	l := lines[rng.IntN(len(lines))]
	return &typed{
		kind: Cloze, answer: Foreign, id: id,
		ask:   "Fill the gap in " + lang.Name + ":",
		clue:  strings.Replace(l.Text, ClozeGap, "_____", 1) + "  (" + e.Prompt + ")",
		grade: e, lang: lang, rules: lang.Defaults, shown: e.Prompt,
		extra: []string{strings.Replace(l.Text, ClozeGap, e.Answers[0], 1), "(" + l.English + ")"},
	}
}
