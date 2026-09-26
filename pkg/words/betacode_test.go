package words

import (
	"reflect"
	"testing"
)

// TestBetaCodeTable pins the Greek typing table: players learn these
// keys, and halpwords-server's list editor offers the same ones.
func TestBetaCodeTable(t *testing.T) {
	letters := map[rune]rune{
		'a': 'α', 'b': 'β', 'g': 'γ', 'd': 'δ', 'e': 'ε', 'z': 'ζ', 'h': 'η', 'q': 'θ',
		'i': 'ι', 'k': 'κ', 'l': 'λ', 'm': 'μ', 'n': 'ν', 'c': 'ξ', 'o': 'ο', 'p': 'π',
		'r': 'ρ', 's': 'σ', 't': 'τ', 'u': 'υ', 'f': 'φ', 'x': 'χ', 'y': 'ψ', 'w': 'ω',
	}
	if got := BetaCodeLetters(); !reflect.DeepEqual(got, letters) {
		t.Errorf("BetaCodeLetters() = %q, want %q", got, letters)
	}
	marks := map[rune]rune{
		')': '̓', '(': '̔', '/': '́', '\\': '̀', '=': '͂', '|': 'ͅ', '+': '̈',
	}
	if got := BetaCodeMarks(); !reflect.DeepEqual(got, marks) {
		t.Errorf("BetaCodeMarks() = %q, want %q", got, marks)
	}
	groups := [][]rune{{'̓', '̔'}, {'́', '̀', '͂'}}
	if got := BetaCodeMarkGroups(); !reflect.DeepEqual(got, groups) {
		t.Errorf("BetaCodeMarkGroups() = %q, want %q", got, groups)
	}
	var alphabet []rune
	for _, k := range BetaCodeChart() {
		if letters[k.Key] != k.Greek {
			t.Errorf("chart: %q types %q, the table says %q", k.Key, k.Greek, letters[k.Key])
		}
		alphabet = append(alphabet, k.Greek)
	}
	if string(alphabet) != "αβγδεζηθικλμνξοπρστυφχψω" {
		t.Errorf("chart order = %q", string(alphabet))
	}
	for key, want := range map[rune]rune{'a': 'α', 'A': 'α', 'W': 'ω'} {
		if g, ok := BetaCodeLetter(key); !ok || g != want {
			t.Errorf("BetaCodeLetter(%q) = %q, %v", key, g, ok)
		}
	}
	for _, key := range "jvJV1)" {
		if g, ok := BetaCodeLetter(key); ok {
			t.Errorf("BetaCodeLetter(%q) = %q, want none", key, g)
		}
	}
	if m, ok := BetaCodeMark('/'); !ok || m != MarkAcute {
		t.Errorf("BetaCodeMark('/') = %q, %v", m, ok)
	}
	if _, ok := BetaCodeMark('a'); ok {
		t.Error("BetaCodeMark('a') is a mark")
	}
	// The results are copies.
	BetaCodeLetters()['a'] = 'x'
	BetaCodeMarkGroups()[0][0] = 'x'
	BetaCodeChart()[0].Greek = 'x'
	if g, _ := BetaCodeLetter('a'); g != 'α' || BetaCodeMarkGroups()[0][0] != MarkSmooth || BetaCodeChart()[0].Greek != 'α' {
		t.Error("changing a returned table changed the table")
	}
}
