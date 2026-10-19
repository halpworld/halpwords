package typing

import (
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/halpworld/halpwords/pkg/words"
)

func field(t *testing.T, code string) *Field {
	t.Helper()
	l, ok := words.Lookup(code)
	if !ok {
		t.Fatal(code)
	}
	return NewField(l)
}

func typeAll(f *Field, s string) {
	for _, r := range s {
		f.Type(r)
	}
}

func TestGreekMode(t *testing.T) {
	tests := []struct{ keys, want string }{
		{"logos", "λογος"},
		{"a)/nqrwpos", "ἄνθρωπος"},
		{"i(/ppos", "ἵππος"},
		{"qa/latta", "θάλαττα"},
		{"yuxh/", "ψυχή"},
		{"dw=ron", "δῶρον"},
		{"tw=|", "τῷ"},
		{"r(", "ῥ"},
		{"Qeos", "Θεος"},
		{"a))", "α"},     // same mark twice toggles it off
		{"a)(", "ἁ"},     // rough replaces smooth
		{"e=", "ε"},      // ε has no circumflex form: rejected
		{"jv", ""},       // no Greek letter
		{"sa s", "σα ς"}, // final sigma before a space
		{"ὁδός", "ὁδός"}, // a real Greek keyboard still works
	}
	for _, tt := range tests {
		f := field(t, "grc")
		typeAll(f, tt.keys)
		if got := f.Text(); got != tt.want {
			t.Errorf("keys %q: got %q, want %q", tt.keys, got, tt.want)
		}
	}
}

func TestGreekFinalSigmaBackspace(t *testing.T) {
	f := field(t, "grc")
	typeAll(f, "logos")
	f.Backspace()
	if got := f.Text(); got != "λογο" {
		t.Fatalf("got %q", got)
	}
	if !f.UsedBackspace {
		t.Fatal("UsedBackspace not set")
	}
}

func TestGreekTabCycle(t *testing.T) {
	f := field(t, "grc")
	typeAll(f, "a")
	want := []string{"ά", "ὰ", "ᾶ", "α"}
	for _, w := range want {
		f.CycleAccent()
		if got := f.Text(); got != w {
			t.Fatalf("got %q, want %q", got, w)
		}
	}
	f = field(t, "grc")
	typeAll(f, "e")
	f.CycleAccent()
	f.CycleAccent()
	f.CycleAccent() // ε has no circumflex: skipped back to plain
	if got := f.Text(); got != "ε" {
		t.Fatalf("got %q, want ε", got)
	}
}

func TestLatinTabCycle(t *testing.T) {
	f := field(t, "fr")
	typeAll(f, "fe")
	for _, w := range []string{"fé", "fè", "fê", "fë", "fe"} {
		f.CycleAccent()
		if got := f.Text(); got != w {
			t.Fatalf("got %q, want %q", got, w)
		}
	}
	f = field(t, "la")
	typeAll(f, "amo")
	f.CycleAccent()
	if got := f.Text(); got != "amō" {
		t.Fatalf("got %q", got)
	}
	f = field(t, "ga")
	typeAll(f, "E")
	f.CycleAccent()
	if got := f.Text(); got != "É" {
		t.Fatalf("got %q", got)
	}
}

func TestCombiningInputIsComposed(t *testing.T) {
	f := field(t, "fr")
	typeAll(f, "é")
	if f.Len() != 1 || f.Text() != "é" {
		t.Fatalf("got %q len %d", f.Text(), f.Len())
	}
}

func TestMaxLen(t *testing.T) {
	f := field(t, "fr")
	for i := 0; i < MaxLen+5; i++ {
		f.Type('a')
	}
	if f.Len() != MaxLen {
		t.Fatalf("len = %d", f.Len())
	}
}

func TestMaxLenIsTheDealLimit(t *testing.T) {
	if MaxLen != words.MaxAnswerRunes {
		t.Fatalf("MaxLen %d, words.MaxAnswerRunes %d", MaxLen, words.MaxAnswerRunes)
	}
}

// The cap counts NFC characters, as words.TooLong does, whichever way the
// letters were typed.
func TestMaxLenAgreesWithTooLong(t *testing.T) {
	for _, tc := range []struct{ code, letter string }{
		{"ga", "é"},    // Irish fada, decomposed
		{"ga", "é"},     // composed
		{"grc", "ά"},   // alpha + acute
		{"grc", "ἄ"},  // alpha + psili + oxia
		{"grc", "ᾗ"}, // eta + dasia + perispomeni + ypogegrammeni
		{"fr", "ê"},
	} {
		f := field(t, tc.code)
		f.Greek = false // type the characters themselves
		for i := 0; i < MaxLen+5; i++ {
			typeAll(f, tc.letter)
		}
		text := f.Text()
		if n := utf8.RuneCountInString(text); n > MaxLen {
			t.Errorf("%q: %d NFC letters typed, limit %d", tc.letter, n, MaxLen)
		}
		if words.TooLong(words.Entry{Answers: []string{text}}) {
			t.Errorf("%q: typed text %d long is TooLong", tc.letter, utf8.RuneCountInString(text))
		}
		if _, ok := composes(tc.letter); ok && f.Len() != MaxLen {
			t.Errorf("%q: field stopped at %d, want %d", tc.letter, f.Len(), MaxLen)
		}
	}
}

func composes(s string) (rune, bool) {
	r := []rune(norm.NFC.String(s))
	return r[0], len(r) == 1
}

func TestFullFieldTakesComposingMark(t *testing.T) {
	f := field(t, "ga")
	typeAll(f, strings.Repeat("a", MaxLen-1)+"e")
	if !f.Type('́') || f.Len() != MaxLen || !strings.HasSuffix(f.Text(), "é") {
		t.Fatalf("len %d text %q", f.Len(), f.Text())
	}
}
