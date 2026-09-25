package puzzle

import (
	"math/rand/v2"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/halpworld/halpwords/pkg/words"
)

// tumbler is a letter lock: one wheel per letter of a word, each with the
// right letter and a few decoys. The article and any punctuation are fixed
// plates between the wheels.
type tumbler struct {
	entry words.Entry
	id    int
	word  string
	lang  *words.Language
	rules words.Rules
	slots [][]string // a fixed plate has one option
	want  []string   // the right option of each slot
	start []int
}

// Wheel limits: shorter words are too easy to guess, and longer ones do not
// fit on the lock.
const (
	MinWheels = 3
	MaxWheels = 10
)

// TumblerOptions is how many letters each wheel has on floor depth.
func TumblerOptions(depth int) int {
	switch {
	case depth <= 2:
		return 3
	case depth <= 5:
		return 4
	}
	return 5
}

// newTumbler returns nil if no word in the deck fits on the lock.
func newTumbler(deck *words.Deck, depth int, lang *words.Language, rng *rand.Rand) *tumbler {
	e, id, ok := deck.NextWhere(func(e words.Entry) bool {
		_, rest := splitArticle(e.Answers[0], lang)
		n := len(Letters(rest))
		return n >= MinWheels && n <= MaxWheels
	})
	if !ok {
		return nil
	}
	word := e.Answers[0]
	article, rest := splitArticle(word, lang)

	// Decoys are letters from the word lists, so they look like the
	// language being learned.
	var pool []string
	seen := map[string]bool{}
	for _, o := range deck.Entries() {
		for _, l := range Letters(o.Answers[0]) {
			if l = strings.ToLower(l); !seen[l] {
				seen[l] = true
				pool = append(pool, l)
			}
		}
	}
	n := TumblerOptions(depth)
	if len(pool) < n {
		return nil
	}

	t := &tumbler{entry: e, id: id, word: word, lang: lang, rules: lang.Defaults}
	plate := func(s string) {
		t.slots = append(t.slots, []string{s})
		t.want = append(t.want, s)
	}
	if article != "" {
		plate(article)
	}
	for _, u := range units(rest) {
		if !isLetter(u) {
			plate(u)
			continue
		}
		// At most one decoy is the right letter with other accents, so
		// the wheel can start on a wrong letter.
		wheel, accented := []string{u}, false
		for _, k := range rng.Perm(len(pool)) {
			if len(wheel) == n {
				break
			}
			d := pool[k]
			if sameLetter(d, u) || hasLetter(wheel, d) || (accented && base(d) == base(u)) {
				continue
			}
			accented = accented || base(d) == base(u)
			wheel = append(wheel, d)
		}
		rng.Shuffle(len(wheel), func(i, j int) { wheel[i], wheel[j] = wheel[j], wheel[i] })
		t.slots = append(t.slots, wheel)
		t.want = append(t.want, u)
	}
	// Every wheel starts on a wrong letter, and not on the right letter
	// with other accents, which would already open the lock.
	t.start = make([]int, len(t.slots))
	for i, w := range t.slots {
		var wrong []int
		for k, l := range w {
			if base(l) != base(t.want[i]) {
				wrong = append(wrong, k)
			}
		}
		if len(wrong) > 0 {
			t.start[i] = wrong[rng.IntN(len(wrong))]
		}
	}
	return t
}

// units splits s into letters (each with its combining marks) and single
// other characters.
func units(s string) []string {
	var out []string
	for _, r := range s {
		if unicode.Is(unicode.Mn, r) && len(out) > 0 {
			out[len(out)-1] += string(r)
			continue
		}
		out = append(out, string(r))
	}
	return out
}

func isLetter(u string) bool {
	r, _ := utf8.DecodeRuneInString(u)
	return unicode.IsLetter(r)
}

// sameLetter compares two letters ignoring case, with a final sigma the
// same as σ.
func sameLetter(a, b string) bool {
	fold := func(s string) string { return strings.ReplaceAll(strings.ToLower(s), "ς", "σ") }
	return fold(a) == fold(b)
}

// base returns a letter in lower case without its marks.
func base(l string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(l)) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return strings.ReplaceAll(b.String(), "ς", "σ")
}

func hasLetter(list []string, l string) bool {
	for _, x := range list {
		if sameLetter(x, l) {
			return true
		}
	}
	return false
}

func (t *tumbler) Kind() Kind      { return Tumbler }
func (t *tumbler) Answer() Answer  { return Dial }
func (t *tumbler) Ask() string     { return "Turn the wheels to spell the " + t.lang.Name + " for:" }
func (t *tumbler) Clue() string    { return t.entry.Prompt }
func (t *tumbler) Tiles() []string { return nil }
func (t *tumbler) Word() int       { return t.id }

func (t *tumbler) Options() [][]string { return t.slots }
func (t *tumbler) Start() []int        { return append([]int(nil), t.start...) }

// Spelled returns the word the wheels show. Slots missing from choice are
// left blank.
func (t *tumbler) Spelled(choice []int) string {
	var b strings.Builder
	for i, k := range choice[:min(len(choice), len(t.slots))] {
		b.WriteString(t.slots[i][k])
	}
	return b.String()
}

func (t *tumbler) Check(a Attempt) Result {
	res := words.Grade(t.Spelled(a.Choice), only(t.entry, t.word), t.lang, t.rules, false)
	return Result{Result: res, Solution: []string{t.entry.Prompt + " = " + t.word}}
}
