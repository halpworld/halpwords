package words

import "unicode"

// This file is the Greek typing table: in Greek mode, Latin keys type
// Greek letters and punctuation keys add marks, in the style of Beta
// Code. The game's text field (internal/typing) uses it, and
// halpwords-server's list editor offers the same keys.

// BetaKey is a key and the Greek letter it types.
type BetaKey struct{ Key, Greek rune }

// The combining marks Greek mode adds.
const (
	MarkSmooth     = '̓' // smooth breathing
	MarkRough      = '̔' // rough breathing
	MarkAcute      = '́'
	MarkGrave      = '̀'
	MarkCircumflex = '͂' // Greek perispomeni
	MarkIotaSub    = 'ͅ' // iota subscript (ypogegrammeni)
	MarkDiaeresis  = '̈'
)

// betaChart is the letters, in alphabet order.
var betaChart = []BetaKey{
	{'a', 'α'}, {'b', 'β'}, {'g', 'γ'}, {'d', 'δ'}, {'e', 'ε'}, {'z', 'ζ'},
	{'h', 'η'}, {'q', 'θ'}, {'i', 'ι'}, {'k', 'κ'}, {'l', 'λ'}, {'m', 'μ'},
	{'n', 'ν'}, {'c', 'ξ'}, {'o', 'ο'}, {'p', 'π'}, {'r', 'ρ'}, {'s', 'σ'},
	{'t', 'τ'}, {'u', 'υ'}, {'f', 'φ'}, {'x', 'χ'}, {'y', 'ψ'}, {'w', 'ω'},
}

var betaLetters = func() map[rune]rune {
	m := make(map[rune]rune, len(betaChart))
	for _, k := range betaChart {
		m[k.Key] = k.Greek
	}
	return m
}()

// betaMarks maps mark keys to combining marks.
var betaMarks = map[rune]rune{
	')': MarkSmooth, '(': MarkRough, '/': MarkAcute, '\\': MarkGrave,
	'=': MarkCircumflex, '|': MarkIotaSub, '+': MarkDiaeresis,
}

// betaMarkGroups are marks that replace each other.
var betaMarkGroups = [][]rune{{MarkSmooth, MarkRough}, {MarkAcute, MarkGrave, MarkCircumflex}}

// BetaCodeChart returns the letter keys in Greek alphabet order, for
// on-screen help. The slice is a copy.
func BetaCodeChart() []BetaKey { return append([]BetaKey(nil), betaChart...) }

// BetaCodeLetter returns the lower-case Greek letter a key types, if
// any. Keys are case-insensitive: 'A' gives 'α' too (the caller makes it
// upper case). j and v have no letter.
func BetaCodeLetter(key rune) (rune, bool) {
	g, ok := betaLetters[unicode.ToLower(key)]
	return g, ok
}

// BetaCodeMark returns the combining mark a key adds to the letter
// before it, if any.
func BetaCodeMark(key rune) (rune, bool) {
	m, ok := betaMarks[key]
	return m, ok
}

// BetaCodeLetters returns every letter key (lower case) and its Greek
// letter. The map is a copy.
func BetaCodeLetters() map[rune]rune { return copyRunes(betaLetters) }

// BetaCodeMarks returns every mark key and its combining mark. The map
// is a copy.
func BetaCodeMarks() map[rune]rune { return copyRunes(betaMarks) }

// BetaCodeMarkGroups returns the groups of marks that replace each
// other on a letter: the breathings, then the accents (acute, grave,
// circumflex, in the order the Tab key cycles them). Adding a mark
// removes the others in its group; adding the same mark again removes
// it. The slices are copies.
func BetaCodeMarkGroups() [][]rune {
	out := make([][]rune, len(betaMarkGroups))
	for i, g := range betaMarkGroups {
		out[i] = append([]rune(nil), g...)
	}
	return out
}

func copyRunes(m map[rune]rune) map[rune]rune {
	out := make(map[rune]rune, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
