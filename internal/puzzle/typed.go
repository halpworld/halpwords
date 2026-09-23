package puzzle

import (
	"math/rand/v2"
	"strings"
	"unicode"

	"github.com/halpworld/halpwords/internal/words"
)

// typed is a puzzle answered by typing one word: Spell, Missing, Anagram and
// Reverse.
type typed struct {
	kind   Kind
	answer Answer
	ask    string
	clue   string
	tiles  []string
	id     int

	// grade holds the accepted answers, graded in lang.
	grade words.Entry
	lang  *words.Language
	// shown starts the solution line: "shown = answer".
	shown string
}

func (t *typed) Kind() Kind      { return t.kind }
func (t *typed) Answer() Answer  { return t.answer }
func (t *typed) Ask() string     { return t.ask }
func (t *typed) Clue() string    { return t.clue }
func (t *typed) Tiles() []string { return t.tiles }
func (t *typed) Word() int       { return t.id }

func (t *typed) Check(a Attempt) Result {
	res := words.Grade(a.Text, t.grade, t.lang, t.lang.Defaults, a.UsedBackspace)
	return Result{Result: res, Solution: []string{t.shown + " = " + res.Expected}}
}

// newSpell asks for a word from its meaning.
func newSpell(e words.Entry, id int, lang *words.Language) *typed {
	return &typed{
		kind: Spell, answer: Foreign, id: id,
		ask:   "Spell in " + lang.Name + ":",
		clue:  e.Prompt,
		grade: e, lang: lang, shown: e.Prompt,
	}
}

// MissingShare is the share of letters a Missing puzzle hides on floor
// depth: a third at first, growing to 60%.
func MissingShare(depth int) float64 {
	return min(0.6, 0.3+0.05*float64(depth))
}

// newMissing shows a word with some letters hidden. It returns nil if there
// is nothing to hide.
func newMissing(e words.Entry, id, depth int, lang *words.Language, rng *rand.Rand) *typed {
	word := e.Answers[0]
	blank := words.Blank(word, MissingShare(depth), rng)
	if !strings.Contains(blank, "_") {
		return nil
	}
	return &typed{
		kind: Missing, answer: Foreign, id: id,
		ask:   "Fill in the " + lang.Name + " word:",
		clue:  e.Prompt + "  →  " + blank,
		grade: only(e, word), lang: lang, shown: e.Prompt,
	}
}

// DecoyDepth is the first floor where chest anagrams have an extra letter
// that is not in the word.
const DecoyDepth = 4

// newAnagram scrambles the letters of a word. The article, if any, stays in
// place. It returns nil if the word is too short to scramble.
func newAnagram(e words.Entry, id int, lock Lock, depth int, all []words.Entry, lang *words.Language, rng *rand.Rand) *typed {
	word := e.Answers[0]
	article, rest := splitArticle(word, lang)
	letters := Letters(rest)
	if len(letters) < 3 || !mixed(letters) {
		return nil
	}
	pattern := []rune(article)
	for _, r := range rest {
		switch {
		case unicode.Is(unicode.Mn, r):
		case unicode.IsLetter(r):
			pattern = append(pattern, '_')
		default:
			pattern = append(pattern, r)
		}
	}
	tiles := append([]string(nil), letters...)
	ask := "Unscramble the " + lang.Name + " word:"
	if lock == Chest && depth >= DecoyDepth {
		if d := decoy(letters, all, rng); d != "" {
			tiles = append(tiles, d)
			ask = "Unscramble the " + lang.Name + " word (one letter is extra):"
		}
	}
	for tries := 0; tries < 20; tries++ {
		rng.Shuffle(len(tiles), func(i, j int) { tiles[i], tiles[j] = tiles[j], tiles[i] })
		if strings.Join(tiles[:len(letters)], "") != strings.Join(letters, "") {
			break
		}
	}
	return &typed{
		kind: Anagram, answer: Foreign, id: id,
		ask:   ask,
		clue:  e.Prompt + "  →  " + string(pattern),
		tiles: tiles,
		grade: only(e, word), lang: lang, shown: e.Prompt,
	}
}

// newReverse shows a foreign word and asks for its meaning in English. Every
// meaning the lists give that word is accepted.
func newReverse(e words.Entry, id int, all []words.Entry) *typed {
	word := e.Answers[0]
	g := words.Entry{Prompt: word, Answers: []string{e.Prompt}}
	for _, o := range all {
		if o.Prompt != e.Prompt && has(o.Answers, word) && !has(g.Answers, o.Prompt) {
			g.Answers = append(g.Answers, o.Prompt)
		}
	}
	return &typed{
		kind: Reverse, answer: Native, id: id,
		ask:   "Type the English for this word:",
		clue:  word,
		grade: g, lang: words.English, shown: word,
	}
}

// only returns e with answer as its only accepted answer, for puzzles that
// show the letters of one answer.
func only(e words.Entry, answer string) words.Entry {
	e.Answers = []string{answer}
	return e
}

func has(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// splitArticle splits a leading article, such as "le " or "l'", from word.
func splitArticle(word string, lang *words.Language) (article, rest string) {
	lower := strings.ToLower(word)
	for _, a := range lang.Articles {
		if len(word) > len(a) && strings.HasPrefix(lower, a) {
			return word[:len(a)], word[len(a):]
		}
	}
	return "", word
}

// Letters splits s into its letters, each with any combining marks that
// follow it. Spaces and punctuation are left out, and a final sigma is
// written σ so it does not give away the end of the word.
func Letters(s string) []string {
	var out []string
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Mn, r) && len(out) > 0:
			out[len(out)-1] += string(r)
		case unicode.IsLetter(r):
			if r == 'ς' {
				r = 'σ'
			}
			out = append(out, string(r))
		}
	}
	return out
}

// mixed reports whether the letters are not all the same, so that some
// shuffle of them differs from the word.
func mixed(letters []string) bool {
	for _, l := range letters[1:] {
		if l != letters[0] {
			return true
		}
	}
	return false
}

// decoy picks a letter from another word that is not in letters.
func decoy(letters []string, all []words.Entry, rng *rand.Rand) string {
	in := map[string]bool{}
	for _, l := range letters {
		in[strings.ToLower(l)] = true
	}
	for tries := 0; tries < 30 && len(all) > 0; tries++ {
		o := Letters(all[rng.IntN(len(all))].Answers[0])
		if len(o) == 0 {
			continue
		}
		if l := o[rng.IntN(len(o))]; !in[strings.ToLower(l)] {
			return l
		}
	}
	return ""
}
