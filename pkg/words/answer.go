package words

import (
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// MaxAnswerRunes is the longest answer the game's typing field takes, in
// characters (a letter with its accents is one). An entry whose answer is
// longer can never be typed, so the game never deals it. The typing field
// and a raid's answers use this limit too.
const MaxAnswerRunes = 60

// TooLong reports whether e's answer is longer than the typing field
// takes, so the game will not deal it. A teacher's list is still good with
// such an entry in it; this is for warning them on save or import. The
// answer that counts is the first, the one shown as the solution.
func TooLong(e Entry) bool {
	return len(e.Answers) > 0 && utf8.RuneCountInString(norm.NFC.String(e.Answers[0])) > MaxAnswerRunes
}

// TooLong returns the entries of l whose answers are too long to type.
func (l *List) TooLong() []Entry {
	var out []Entry
	for _, e := range l.Entries {
		if TooLong(e) {
			out = append(out, e)
		}
	}
	return out
}
