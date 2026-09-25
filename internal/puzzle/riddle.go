package puzzle

import (
	"bufio"
	"bytes"
	"math/rand/v2"
	"strings"
	"sync"

	"github.com/halpworld/halpwords/assets"
	"github.com/halpworld/halpwords/internal/words"
)

// Riddles returns the riddle bank: riddles in English, by the English word
// they describe (lower case). The hero answers in the language they learn,
// so one bank serves every word list with those English words.
var Riddles = sync.OnceValue(func() map[string][]string {
	return ParseRiddles(assets.Riddles)
})

// ParseRiddles reads "english = riddle" lines. Lines starting with # are
// comments.
func ParseRiddles(data []byte) map[string][]string {
	out := map[string][]string{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		word, riddle, ok := strings.Cut(line, "=")
		word, riddle = strings.ToLower(strings.TrimSpace(word)), strings.TrimSpace(riddle)
		if ok && word != "" && riddle != "" {
			out[word] = append(out[word], riddle)
		}
	}
	return out
}

// newRiddle asks for a word from a riddle about it. It returns nil if no
// word in the deck has a riddle.
func newRiddle(deck *words.Deck, lang *words.Language, rng *rand.Rand) *typed {
	bank := Riddles()
	e, id, ok := deck.NextWhere(func(e words.Entry) bool {
		return len(bank[strings.ToLower(e.Prompt)]) > 0
	})
	if !ok {
		return nil
	}
	rs := bank[strings.ToLower(e.Prompt)]
	return &typed{
		kind: Riddle, answer: Foreign, id: id,
		ask:   "Answer the riddle in " + lang.Name + ":",
		clue:  rs[rng.IntN(len(rs))],
		grade: e, lang: lang, rules: lang.Defaults, shown: e.Prompt,
	}
}
