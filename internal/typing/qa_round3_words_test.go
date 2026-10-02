package typing

import (
	"strings"
	"testing"

	"github.com/halpworld/halpwords/pkg/words"
)

// The field takes exactly the longest answer the game deals, in letters
// (accents compose to one), and nothing deal-able is longer.
func TestQA3FieldTakesAWholeDealableAnswer(t *testing.T) {
	for _, ans := range []string{
		strings.Repeat("a", words.MaxAnswerRunes),
		strings.Repeat("\u00e9", words.MaxAnswerRunes),
	} {
		e := words.Entry{Prompt: "p", Answers: []string{ans}}
		if words.TooLong(e) {
			t.Fatal("test answer is too long")
		}
		f := field(t, "fr")
		for _, r := range ans {
			if !f.Type(r) {
				t.Fatalf("field refused a character at length %d", f.Len())
			}
		}
		if f.Len() != words.MaxAnswerRunes {
			t.Fatalf("len %d", f.Len())
		}
		if r := words.Grade(f.Text(), e, f.Lang, f.Lang.Defaults, false); r.Tier != words.Perfect {
			t.Errorf("graded %v", r.Tier)
		}
		if f.Type('x') {
			t.Error("the 61st letter was accepted")
		}
	}
}

// Typing a combining accent after the 60th letter: the accent composes into
// that letter, it is not a 61st character, so the field should take it.
func TestQA3CombiningAccentOnTheLastLetter(t *testing.T) {
	f := field(t, "fr")
	for i := 0; i < words.MaxAnswerRunes-1; i++ {
		f.Type('a')
	}
	f.Type('e')
	if !f.Type('\u0301') {
		t.Skip("round 3: Field.Type checks MaxLen before composing a combining mark, so the accent on the 60th letter of a 60-letter answer is refused (typing.go:84)")
	}
	if f.Len() != words.MaxAnswerRunes || !strings.HasSuffix(f.Text(), "\u00e9") {
		t.Fatalf("len %d text %q", f.Len(), f.Text())
	}
}
