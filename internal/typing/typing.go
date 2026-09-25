// Package typing implements the text entry field used in battles and puzzles,
// including the Tab accent helper and the Greek (Beta Code style) input mode.
package typing

import (
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/halpworld/halpwords/pkg/words"
)

// MaxLen is the longest answer the field accepts, in characters.
const MaxLen = 40

// betaCode maps Latin keys to Greek letters in Greek mode.
var betaCode = map[rune]rune{
	'a': 'α', 'b': 'β', 'g': 'γ', 'd': 'δ', 'e': 'ε', 'z': 'ζ', 'h': 'η', 'q': 'θ',
	'i': 'ι', 'k': 'κ', 'l': 'λ', 'm': 'μ', 'n': 'ν', 'c': 'ξ', 'o': 'ο', 'p': 'π',
	'r': 'ρ', 's': 'σ', 't': 'τ', 'u': 'υ', 'f': 'φ', 'x': 'χ', 'y': 'ψ', 'w': 'ω',
}

// BetaCodeChart lists the Greek mode keys in alphabet order, for on-screen help.
var BetaCodeChart = []struct{ Key, Greek rune }{
	{'a', 'α'}, {'b', 'β'}, {'g', 'γ'}, {'d', 'δ'}, {'e', 'ε'}, {'z', 'ζ'},
	{'h', 'η'}, {'q', 'θ'}, {'i', 'ι'}, {'k', 'κ'}, {'l', 'λ'}, {'m', 'μ'},
	{'n', 'ν'}, {'c', 'ξ'}, {'o', 'ο'}, {'p', 'π'}, {'r', 'ρ'}, {'s', 'σ'},
	{'t', 'τ'}, {'u', 'υ'}, {'f', 'φ'}, {'x', 'χ'}, {'y', 'ψ'}, {'w', 'ω'},
}

// Combining marks used in Greek mode.
const (
	smooth     = '̓'
	rough      = '̔'
	acute      = '́'
	grave      = '̀'
	circumflex = '͂'
	iotaSub    = 'ͅ'
	diaeresis  = '̈'
)

// markKeys maps Beta Code diacritic keys to combining marks.
var markKeys = map[rune]rune{
	')': smooth, '(': rough, '/': acute, '\\': grave, '=': circumflex, '|': iotaSub, '+': diaeresis,
}

// Marks that replace each other.
var markGroups = [][]rune{{smooth, rough}, {acute, grave, circumflex}}

// Field is a single-line text entry.
type Field struct {
	Lang          *words.Language
	Greek         bool // Greek input mode: Latin keys type Greek letters
	UsedBackspace bool

	runes []rune
}

// NewField returns an empty field for lang. Greek mode is on for Greek.
func NewField(lang *words.Language) *Field {
	return &Field{Lang: lang, Greek: lang.Script == words.ScriptGreek}
}

// Reset empties the field.
func (f *Field) Reset() {
	f.runes = f.runes[:0]
	f.UsedBackspace = false
}

// Len returns the number of characters typed.
func (f *Field) Len() int { return len(f.runes) }

// Text returns the typed text in NFC, with final sigma applied.
func (f *Field) Text() string {
	out := make([]rune, len(f.runes))
	copy(out, f.runes)
	for i, r := range out {
		if r == 'σ' && (i == len(out)-1 || !unicode.IsLetter(out[i+1])) {
			out[i] = 'ς'
		}
	}
	return norm.NFC.String(string(out))
}

// Type handles a typed character and reports whether it was accepted.
func (f *Field) Type(r rune) bool {
	if unicode.IsControl(r) {
		return false
	}
	if f.Greek {
		if m, ok := markKeys[r]; ok {
			return f.addMark(m)
		}
		if g, ok := betaCode[unicode.ToLower(r)]; ok {
			if unicode.IsUpper(r) {
				g = unicode.ToUpper(g)
			}
			r = g
		} else if r < 0x80 && unicode.IsLetter(r) {
			return false // j and v have no Greek letter
		}
	}
	if len(f.runes) >= MaxLen {
		return false
	}
	// Keep everything precomposed so Backspace removes whole letters.
	if unicode.Is(unicode.Mn, r) && len(f.runes) > 0 {
		c := []rune(norm.NFC.String(string(f.runes[len(f.runes)-1]) + string(r)))
		if len(c) == 1 {
			f.runes[len(f.runes)-1] = c[0]
			return true
		}
		return false
	}
	f.runes = append(f.runes, r)
	return true
}

// Backspace deletes the last character.
func (f *Field) Backspace() bool {
	if len(f.runes) == 0 {
		return false
	}
	f.runes = f.runes[:len(f.runes)-1]
	f.UsedBackspace = true
	return true
}

// CycleAccent changes the last letter to its next accented form (the Tab key).
// In Greek mode it cycles the accent: none → acute → grave → circumflex.
func (f *Field) CycleAccent() bool {
	if len(f.runes) == 0 {
		return false
	}
	last := f.runes[len(f.runes)-1]
	if f.Greek {
		base, marks := decompose(last)
		next := map[rune]rune{0: acute, acute: grave, grave: circumflex, circumflex: 0}
		cur := rune(0)
		for _, m := range marks {
			if m == acute || m == grave || m == circumflex {
				cur = m
			}
		}
		for n := next[cur]; ; n = next[n] {
			ms := without(marks, markGroups[1])
			if n != 0 {
				ms = append(ms, n)
			}
			if c, ok := compose(base, ms); ok {
				f.runes[len(f.runes)-1] = c
				return c != last
			}
			if n == cur {
				return false
			}
		}
	}
	cycle, i, ok := f.Lang.AccentCycle(last)
	if !ok {
		return false
	}
	f.runes[len(f.runes)-1] = cycle[(i+1)%len(cycle)]
	return true
}

// addMark applies a combining mark to the last letter. Typing the same mark
// again removes it; a mark from the same group replaces the old one.
func (f *Field) addMark(m rune) bool {
	if len(f.runes) == 0 {
		return false
	}
	base, marks := decompose(f.runes[len(f.runes)-1])
	had := false
	for _, x := range marks {
		if x == m {
			had = true
		}
	}
	for _, g := range markGroups {
		for _, x := range g {
			if x == m {
				marks = without(marks, g)
			}
		}
	}
	marks = without(marks, []rune{m})
	if !had {
		marks = append(marks, m)
	}
	c, ok := compose(base, marks)
	if !ok {
		return false
	}
	f.runes[len(f.runes)-1] = c
	return true
}

func decompose(r rune) (base rune, marks []rune) {
	d := []rune(norm.NFD.String(string(r)))
	return d[0], d[1:]
}

// compose builds a single precomposed character, if Unicode has one.
func compose(base rune, marks []rune) (rune, bool) {
	c := []rune(norm.NFC.String(string(append([]rune{base}, marks...))))
	if len(c) != 1 {
		return 0, false
	}
	return c[0], true
}

func without(marks, remove []rune) []rune {
	out := make([]rune, 0, len(marks))
	for _, m := range marks {
		keep := true
		for _, r := range remove {
			if m == r {
				keep = false
			}
		}
		if keep {
			out = append(out, m)
		}
	}
	return out
}
