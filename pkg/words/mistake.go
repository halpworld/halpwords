package words

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Mistake is the kind of mistake in a wrong answer, for feedback and the
// Grimoire's statistics.
type Mistake int

const (
	NoMistake      Mistake = iota
	AccentMistake          // right letters, wrong or missing accents
	DoubleMistake          // a double letter written single, or a single one doubled
	SwapMistake            // two letters next to each other swapped
	MissingMistake         // a letter left out
	ExtraMistake           // a letter too many
	LetterMistake          // one wrong letter
	WrongWord              // anything else, or nothing typed

	NumMistakes
)

var mistakeNames = [NumMistakes]string{"", "accents", "double letters", "swapped letters",
	"missing letters", "extra letters", "wrong letters", "wrong words"}

func (m Mistake) String() string {
	if m < 0 || m >= NumMistakes {
		return ""
	}
	return mistakeNames[m]
}

// Tip is a short piece of advice about a mistake, or "" for none.
func (m Mistake) Tip() string {
	switch m {
	case AccentMistake:
		return "Watch the accents!"
	case DoubleMistake:
		return "Check the double letters."
	case SwapMistake:
		return "Two letters are swapped."
	case MissingMistake:
		return "A letter is missing."
	case ExtraMistake:
		return "There is a letter too many."
	case LetterMistake:
		return "One letter is wrong."
	}
	return ""
}

// Classify works out the kind of mistake in typed, compared with the answer
// it was closest to. Capitals and the kind of apostrophe don't matter, and
// neither does leaving out an article lang has (lang may be nil).
func Classify(typed, expected string, lang *Language) Mistake {
	m := classify(typed, expected)
	if m == WrongWord && lang != nil {
		t, e := normalize(typed, Rules{}), normalize(expected, Rules{})
		if bare := stripArticle(e, lang); bare != e {
			m = classify(stripArticle(t, lang), bare)
		}
	}
	return m
}

func classify(typed, expected string) Mistake {
	rules := Rules{}
	t, e := normalize(typed, rules), normalize(expected, rules)
	switch {
	case t == e:
		return NoMistake
	case t == "":
		return WrongWord
	case stripMarks(t, true, true) == stripMarks(e, true, true):
		return AccentMistake
	}
	// Compare without marks, so an accent slip on top of a typo still
	// counts as the typo.
	tr, er := []rune(stripMarks(t, true, true)), []rune(stripMarks(e, true, true))
	switch len(tr) - len(er) {
	case 0:
		diff := -1
		for i := range tr {
			if tr[i] != er[i] {
				if diff >= 0 {
					if i == diff+1 && tr[diff] == er[i] && tr[i] == er[diff] && same(tr[i+1:], er[i+1:]) {
						return SwapMistake
					}
					return WrongWord
				}
				diff = i
			}
		}
		return LetterMistake
	case -1:
		if i, ok := oneOut(er, tr); ok {
			if (i > 0 && er[i-1] == er[i]) || (i+1 < len(er) && er[i+1] == er[i]) {
				return DoubleMistake
			}
			return MissingMistake
		}
	case 1:
		if i, ok := oneOut(tr, er); ok {
			if (i > 0 && tr[i-1] == tr[i]) || (i+1 < len(tr) && tr[i+1] == tr[i]) {
				return DoubleMistake
			}
			return ExtraMistake
		}
	}
	return WrongWord
}

// oneOut reports whether short is long with one letter taken out, and
// which.
func oneOut(long, short []rune) (int, bool) {
	i := 0
	for i < len(short) && long[i] == short[i] {
		i++
	}
	return i, same(long[i+1:], short[i:])
}

func same(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Difficulty rates how hard a word is to spell, from its length and its
// letters with marks. Most words score 3 to 12.
func Difficulty(e Entry) float64 {
	if len(e.Answers) == 0 {
		return 0
	}
	a := e.Answers[0]
	d := float64(utf8.RuneCountInString(a))
	for _, r := range a {
		if utf8.RuneCountInString(norm.NFD.String(string(r))) > 1 || r == 'œ' || r == 'æ' {
			d += 1.5 // é, ā, ἀ...
		}
	}
	if strings.ContainsAny(a, " -") {
		d += 1
	}
	return d
}
