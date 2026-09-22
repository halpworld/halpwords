package words

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Tier is how well an answer matched, from worst to best.
type Tier int

const (
	Miss       Tier = iota // wrong, or a strict mark error
	Graze                  // one letter off (or two letters swapped)
	AccentSlip             // right letters, wrong or missing marks
	Correct                // right, but fixed with backspace
	Perfect                // right first time
)

func (t Tier) String() string {
	return [...]string{"MISS", "GRAZE", "ACCENT SLIP", "CORRECT", "PERFECT"}[t]
}

// Result is the outcome of grading an answer.
type Result struct {
	Tier Tier
	// Expected is the accepted answer closest to what was typed.
	Expected string
	// MarkError is set when the only problem was diacritics.
	MarkError bool
}

const (
	combSmooth = '̓'
	combRough  = '̔'
	combKoro   = '̓' // koronis, canonically a smooth breathing
)

// Grade compares typed against the accepted answers of e.
// usedBackspace downgrades a Perfect to Correct.
func Grade(typed string, e Entry, lang *Language, rules Rules, usedBackspace bool) Result {
	in := normalize(typed, rules)
	best := Result{Tier: Miss, Expected: e.Answers[0]}
	if in == "" {
		return best
	}
	prep := func(s string) string {
		if !rules.ArticlesRequired {
			s = stripArticle(s, lang)
		}
		return s
	}
	for _, a := range e.Answers {
		ans := normalize(a, rules)
		r := Result{Tier: Miss, Expected: a}

		// Level 1: equal once ignored marks are removed.
		l1 := func(s string) string {
			return stripMarks(prep(s), rules.Accents == Ignore, rules.Breathings == Ignore)
		}
		// Level 2: equal once every non-strict mark is removed.
		l2 := func(s string) string {
			return stripMarks(prep(s), rules.Accents != Strict, rules.Breathings != Strict)
		}
		// Level 3: equal once every mark is removed.
		l3 := func(s string) string { return stripMarks(prep(s), true, true) }

		switch {
		case l1(in) == l1(ans):
			r.Tier = Perfect
			if usedBackspace {
				r.Tier = Correct
			}
		case l2(in) == l2(ans):
			r.Tier, r.MarkError = AccentSlip, true
		case l3(in) == l3(ans):
			r.Tier, r.MarkError = Miss, true // strict marks
		default:
			x, y := l2(in), l2(ans)
			if utf8.RuneCountInString(y) >= 4 && editDistance(x, y) == 1 {
				r.Tier = Graze
			}
		}
		if r.Tier > best.Tier || (r.Tier == best.Tier && r.MarkError && !best.MarkError) {
			best = r
		}
	}
	return best
}

// normalize applies the always-on normalisation: NFC, collapsed spaces,
// straight apostrophes, σ for final sigma, and lower case unless
// case-sensitive.
func normalize(s string, rules Rules) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.Map(func(r rune) rune {
		switch r {
		case '’', '‘', 'ʼ', '`', '´':
			return '\''
		case 'ς', 'ϲ':
			return 'σ'
		}
		return r
	}, s)
	if !rules.CaseSensitive {
		s = strings.ToLower(s)
	}
	return norm.NFC.String(s)
}

func stripArticle(s string, lang *Language) string {
	for _, a := range lang.Articles {
		if len(s) > len(a) && strings.HasPrefix(s, a) {
			return s[len(a):]
		}
	}
	return s
}

// stripMarks removes accent-class and/or breathing-class diacritics.
func stripMarks(s string, accents, breathings bool) string {
	if !accents && !breathings {
		return s
	}
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		switch {
		case r == combSmooth || r == combRough || r == combKoro:
			if breathings {
				continue
			}
		case unicode.Is(unicode.Mn, r):
			if accents {
				continue
			}
		case accents && (r == 'œ' || r == 'Œ'):
			b.WriteString("oe")
			continue
		case accents && (r == 'æ' || r == 'Æ'):
			b.WriteString("ae")
			continue
		}
		b.WriteRune(r)
	}
	return norm.NFC.String(b.String())
}

// editDistance is the optimal string alignment distance: insertions,
// deletions, substitutions and swaps of neighbouring letters each cost 1.
func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	d := make([][]int, len(ra)+1)
	for i := range d {
		d[i] = make([]int, len(rb)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(ra)][len(rb)]
}
