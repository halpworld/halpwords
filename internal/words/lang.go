// Package words loads word lists and grades typed answers.
package words

import "unicode"

// Script is the writing system a language is typed in.
type Script int

const (
	ScriptLatin Script = iota
	ScriptGreek
)

// Strictness controls how a class of diacritics is graded.
type Strictness int

const (
	Ignore  Strictness = iota // a missing or wrong mark is not an error
	Reduced                   // a mark error still hits, for reduced credit
	Strict                    // a mark error is a miss
)

func (s Strictness) String() string {
	switch s {
	case Ignore:
		return "ignore"
	case Reduced:
		return "reduced credit"
	default:
		return "strict"
	}
}

// Rules are the per-language grading settings.
type Rules struct {
	Accents          Strictness // accents, fadas, macrons, cedillas, diaereses, iota subscript
	Breathings       Strictness // Greek smooth/rough breathing
	ArticlesRequired bool       // French: "le chien" vs "chien"
	CaseSensitive    bool
}

// Language describes a target language.
type Language struct {
	Code     string
	Name     string
	Script   Script
	Articles []string // prefixes stripped when articles are optional
	// Accents maps a base letter to the variants the Tab key cycles through.
	Accents  map[rune][]rune
	Defaults Rules
}

// Languages lists the supported languages in menu order.
var Languages = []*Language{
	{
		Code:     "fr",
		Name:     "French",
		Script:   ScriptLatin,
		Articles: []string{"le ", "la ", "les ", "l'", "un ", "une ", "des "},
		Accents: map[rune][]rune{
			'a': []rune("àâ"), 'e': []rune("éèêë"), 'i': []rune("îï"),
			'o': []rune("ôœ"), 'u': []rune("ùûü"), 'c': []rune("ç"), 'y': []rune("ÿ"),
		},
		Defaults: Rules{Accents: Reduced, Breathings: Ignore},
	},
	{
		Code:   "la",
		Name:   "Latin",
		Script: ScriptLatin,
		Accents: map[rune][]rune{
			'a': []rune("ā"), 'e': []rune("ē"), 'i': []rune("ī"),
			'o': []rune("ō"), 'u': []rune("ū"), 'y': []rune("ȳ"),
		},
		Defaults: Rules{Accents: Ignore, Breathings: Ignore},
	},
	{
		Code:     "grc",
		Name:     "Ancient Greek",
		Script:   ScriptGreek,
		Defaults: Rules{Accents: Ignore, Breathings: Ignore},
	},
	{
		Code:   "ga",
		Name:   "Irish",
		Script: ScriptLatin,
		Accents: map[rune][]rune{
			'a': []rune("á"), 'e': []rune("é"), 'i': []rune("í"),
			'o': []rune("ó"), 'u': []rune("ú"),
		},
		Defaults: Rules{Accents: Reduced, Breathings: Ignore},
	},
}

// English grades answers typed in English, for puzzles that show a foreign
// word and ask what it means. It is not a language to learn, so it is not in
// Languages. Leading articles, "to" and "I" are optional, so "the dog" and
// "dog", or "I love" and "love", are both right.
var English = &Language{
	Code:     "en",
	Name:     "English",
	Script:   ScriptLatin,
	Articles: []string{"the ", "a ", "an ", "to ", "i "},
	Defaults: Rules{Accents: Ignore, Breathings: Ignore},
}

// Lookup returns the language with the given code.
func Lookup(code string) (*Language, bool) {
	for _, l := range Languages {
		if l.Code == code {
			return l, true
		}
	}
	return nil, false
}

// AccentCycle returns the Tab cycle containing r (base letter first) and r's
// position in it, preserving r's case. ok is false if r has no variants.
func (l *Language) AccentCycle(r rune) (cycle []rune, index int, ok bool) {
	upper := unicode.IsUpper(r)
	lr := unicode.ToLower(r)
	for base, vs := range l.Accents {
		c := append([]rune{base}, vs...)
		for i, v := range c {
			if v == lr {
				if upper {
					for j := range c {
						c[j] = unicode.ToUpper(c[j])
					}
				}
				return c, i, true
			}
		}
	}
	return nil, 0, false
}
