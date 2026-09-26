package puzzle

import (
	"bufio"
	"bytes"
	"math/rand/v2"
	"strings"
	"sync"

	"github.com/halpworld/halpwords/assets"
	"github.com/halpworld/halpwords/pkg/words"
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

// newRiddle asks for a word from a riddle about it, from the riddle bank or
// the riddles an AI wrote. It returns nil if no word in the deck has a
// riddle.
func newRiddle(deck dealer, lang *words.Language, gen *Generated, rng *rand.Rand) *typed {
	bank := Riddles()
	riddles := func(e words.Entry) []string {
		k := strings.ToLower(e.Prompt)
		rs := bank[k]
		if gen != nil && len(gen.Riddles[k]) > 0 {
			rs = append(append([]string(nil), rs...), gen.Riddles[k]...)
		}
		return rs
	}
	e, id, ok := deck.NextWhere(func(e words.Entry) bool { return len(riddles(e)) > 0 })
	if !ok {
		return nil
	}
	rs := riddles(e)
	return &typed{
		kind: Riddle, answer: Foreign, id: id,
		ask:   "Answer the riddle in " + lang.Name + ":",
		clue:  rs[rng.IntN(len(rs))],
		grade: e, lang: lang, rules: lang.Defaults, shown: e.Prompt,
	}
}
