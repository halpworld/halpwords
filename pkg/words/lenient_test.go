package words

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func TestParseLenient(t *testing.T) {
	long := strings.Repeat("a", MaxAnswerRunes+1)
	tests := []struct {
		name    string
		in      string
		words   []string // prompts kept
		lines   []int    // lines skipped
		reasons []string // substrings, in order
		lang    string
	}{
		{"clean", "language: fr\ndog = chien\ncat = chat\n", []string{"dog", "cat"}, nil, nil, "fr"},
		{"no language is fine", "dog = chien\n", []string{"dog"}, nil, nil, ""},
		{"no equals", "dog = chien\nthe cat chat\n", []string{"dog"}, []int{2}, []string{"no '='"}, ""},
		{"empty answer", "dog =\ncat = | \n", nil, []int{1, 2}, []string{"nothing after", "nothing after"}, ""},
		{"empty prompt", " = chien\n", nil, []int{1}, []string{"nothing before"}, ""},
		{"colon not a setting", "dog: chien\n", nil, []int{1}, []string{`"dog" is not a setting`}, ""},
		{"unknown setting", "colour: red\n", nil, []int{1}, []string{`"colour" is not a setting`}, ""},
		{"x- ignored", "x-foo: bar\ndog = chien\n", []string{"dog"}, nil, nil, ""},
		{"bad version", "version: abc\ndog = chien\n", []string{"dog"}, []int{1}, []string{"not a whole number"}, ""},
		{"unknown language", "language: xx\ndog = chien\n", []string{"dog"}, []int{1}, []string{"unknown language"}, ""},
		{"language name", "language: French\ndog = chien\n", []string{"dog"}, nil, nil, "fr"},
		{"too long", "dog = " + long + "\ncat = chat\n", []string{"cat"}, []int{1}, []string{"too long"}, ""},
		{"just fits", "dog = " + strings.Repeat("é", MaxAnswerRunes) + "\n", []string{"dog"}, nil, nil, ""},
		{"duplicate", "dog = chien\nDog = clebe\ncat = chat\n", []string{"dog", "cat"}, []int{2}, []string{"already on line 1"}, ""},
		{"gap ok", "dog = chien\n>> Le ___ aboie. | chien\n", []string{"dog"}, nil, nil, ""},
		{"gap answer not in list", "dog = chien\n>> Le ___ aboie. | chat\n", []string{"dog"}, []int{2}, []string{"not one of the list's answers"}, ""},
		{"gap before its word", ">> Le ___ aboie. | chien\ndog = chien\n", []string{"dog"}, nil, nil, ""},
		{"gap no bar", "dog = chien\n>> Le ___ aboie.\n", []string{"dog"}, []int{2}, []string{"expected"}, ""},
		{"gap no gap", "dog = chien\n>> Le chien aboie. | chien\n", []string{"dog"}, []int{2}, []string{"needs one gap"}, ""},
		{"several problems in order", "a = b\nbad\n>> x | y\nc =\n", []string{"a"}, []int{2, 3, 4}, nil, ""},
		{"BOM", "\uFEFFlanguage: fr\ndog = chien\n", []string{"dog"}, nil, nil, "fr"},
		{"tabs", "dog\tchien\ncat\tchat\tmiaou\n", []string{"dog", "cat"}, nil, nil, ""},
		{"windows line endings", "language: fr\r\ndog = chien\r\n\r\nbad\r\ncat = chat\r\n", []string{"dog", "cat"}, []int{4}, nil, "fr"},
		{"comments and groups", "# hi\n## animals\ndog = chien\n", []string{"dog"}, nil, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, probs := ParseLenient(strings.NewReader(tt.in), "t.txt")
			var got []string
			for _, e := range l.Entries {
				got = append(got, e.Prompt)
			}
			if !slices.Equal(got, tt.words) {
				t.Errorf("words = %v, want %v", got, tt.words)
			}
			var lines []int
			for _, p := range probs {
				lines = append(lines, p.Line)
			}
			if !slices.Equal(lines, tt.lines) {
				t.Errorf("problem lines = %v, want %v (%v)", lines, tt.lines, probs)
			}
			for i, r := range tt.reasons {
				if i < len(probs) && !strings.Contains(probs[i].Reason, r) {
					t.Errorf("reason %d = %q, want it to contain %q", i, probs[i].Reason, r)
				}
			}
			if l.Language != tt.lang {
				t.Errorf("language = %q, want %q", l.Language, tt.lang)
			}
		})
	}
}

func TestParseLenientNFCAndText(t *testing.T) {
	// "e" + combining acute becomes one letter, and counts as one for the limit.
	l, probs := ParseLenient(strings.NewReader("tea = thé\n  bad line  \n"), "t.txt")
	if len(probs) != 1 || probs[0].Text != "bad line" || probs[0].Line != 2 {
		t.Fatalf("problems = %+v", probs)
	}
	if got := l.Entries[0].Answers[0]; got != "thé" {
		t.Errorf("answer = %q, not NFC", got)
	}
	if s := probs[0].String(); !strings.HasPrefix(s, "line 2: no '='") {
		t.Errorf("String = %q", s)
	}
	if l.Title != "t" {
		t.Errorf("title = %q", l.Title)
	}
}

// A file Parse accepts has no problems, and the same words.
func TestParseLenientAgreesWithParse(t *testing.T) {
	in := "title: X\nlanguage: fr\n## a\ndog = le chien\nfriend = l'ami | l'amie\n>> Mon ___ aboie. | le chien\n"
	want, err := Parse(strings.NewReader(in), "x.txt")
	if err != nil {
		t.Fatal(err)
	}
	got, probs := ParseLenient(strings.NewReader(in), "x.txt")
	if len(probs) != 0 || Format(got) != Format(want) {
		t.Errorf("probs %v\n%s\nvs\n%s", probs, Format(got), Format(want))
	}
}

func TestParseLenientEmpty(t *testing.T) {
	l, probs := ParseLenient(strings.NewReader(""), "e.txt")
	if l == nil || len(l.Entries) != 0 || len(probs) != 0 {
		t.Errorf("got %+v %v", l, probs)
	}
}

// The 8-word example in the README's "Your own word lists" has no problems.
func TestREADMEExample(t *testing.T) {
	data, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Skip(err)
	}
	s := string(data)
	_, s, ok := strings.Cut(s, "## Your own word lists")
	if !ok {
		t.Fatal("no such section")
	}
	_, s, _ = strings.Cut(s, "```text\n")
	s, _, _ = strings.Cut(s, "```")
	l, probs := ParseLenient(strings.NewReader(s), "homework.txt")
	if len(probs) != 0 || len(l.Entries) != 8 || l.Language != "fr" {
		t.Errorf("%d words, language %q, problems %v", len(l.Entries), l.Language, probs)
	}
	if _, err := Parse(strings.NewReader(s), "homework.txt"); err != nil {
		t.Error(err)
	}
}
