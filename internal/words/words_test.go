package words

import (
	"strings"
	"testing"

	"github.com/halpworld/halpwords/assets"
)

func lang(t *testing.T, code string) *Language {
	t.Helper()
	l, ok := Lookup(code)
	if !ok {
		t.Fatalf("no language %q", code)
	}
	return l
}

func TestStarterListsParse(t *testing.T) {
	lists, err := LoadFS(assets.Words, "words")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, l := range lists {
		seen[l.Language] = true
		lg := lang(t, l.Language)
		// Every starter answer must grade Perfect against itself.
		for _, e := range l.Entries {
			for _, a := range e.Answers {
				if r := Grade(a, e, lg, Rules{Accents: Strict, Breathings: Strict, ArticlesRequired: true}, false); r.Tier != Perfect {
					t.Errorf("%s: %q graded %v against itself", l.Source, a, r.Tier)
				}
			}
		}
	}
	for _, code := range []string{"fr", "la", "grc", "ga"} {
		if !seen[code] {
			t.Errorf("no starter list for %s", code)
		}
	}
}

func TestParse(t *testing.T) {
	src := "# hi\ntitle: Test\nlanguage: fr\n\n## pets\ndog = le chien\nfriend = l'ami | l’amie\n"
	l, err := Parse(strings.NewReader(src), "t.txt")
	if err != nil {
		t.Fatal(err)
	}
	if l.Title != "Test" || len(l.Entries) != 2 {
		t.Fatalf("got %+v", l)
	}
	if e := l.Entries[1]; e.Tag != "pets" || len(e.Answers) != 2 {
		t.Fatalf("got %+v", e)
	}
	for _, bad := range []string{
		"language: fr\ndog",
		"language: xx\ndog = x",
		"dog = le chien",
		"language: fr\ncolour: red\ndog = x",
		"language: fr\n = x",
	} {
		if _, err := Parse(strings.NewReader(bad), "bad.txt"); err == nil {
			t.Errorf("Parse(%q) succeeded, want error", bad)
		}
	}
}

func TestGrade(t *testing.T) {
	fr, la, grc, ga := lang(t, "fr"), lang(t, "la"), lang(t, "grc"), lang(t, "ga")
	e := func(answers ...string) Entry { return Entry{Prompt: "x", Answers: answers} }

	tests := []struct {
		name  string
		typed string
		entry Entry
		lang  *Language
		rules Rules
		bs    bool
		want  Tier
	}{
		{"fr exact", "le château", e("le château"), fr, fr.Defaults, false, Perfect},
		{"fr backspace", "le château", e("le château"), fr, fr.Defaults, true, Correct},
		{"fr no article", "château", e("le château"), fr, fr.Defaults, false, Perfect},
		{"fr article required", "château", e("le château"), fr, Rules{Accents: Reduced, ArticlesRequired: true}, false, Miss},
		{"fr elision", "oiseau", e("l'oiseau"), fr, fr.Defaults, false, Perfect},
		{"fr curly apostrophe", "l’oiseau", e("l'oiseau"), fr, fr.Defaults, false, Perfect},
		{"fr missing accent", "chateau", e("le château"), fr, fr.Defaults, false, AccentSlip},
		{"fr strict accent", "chateau", e("le château"), fr, Rules{Accents: Strict}, false, Miss},
		{"fr ignore accent", "chateau", e("le château"), fr, Rules{Accents: Ignore}, false, Perfect},
		{"fr oe ligature", "la soeur", e("la sœur"), fr, fr.Defaults, false, AccentSlip},
		{"fr case", "LE CHIEN", e("le chien"), fr, fr.Defaults, false, Perfect},
		{"fr graze swap", "le chein", e("le chien"), fr, fr.Defaults, false, Graze},
		{"fr two errors", "le shein", e("le chien"), fr, fr.Defaults, false, Miss},
		{"fr graze sub", "le chiem", e("le chien"), fr, fr.Defaults, false, Graze},
		{"fr alternative", "l'amie", e("l'ami", "l'amie"), fr, fr.Defaults, false, Perfect},
		{"fr wrong", "le chat", e("le chien"), fr, fr.Defaults, false, Miss},
		{"fr empty", "  ", e("le chien"), fr, fr.Defaults, false, Miss},
		{"la macron ignored", "amicus", e("amīcus"), la, la.Defaults, false, Perfect},
		{"la macron strict", "amicus", e("amīcus"), la, Rules{Accents: Strict}, false, Miss},
		{"la short word no graze", "rez", e("rēx"), la, la.Defaults, false, Miss},
		{"ga fada reduced", "cailin", e("cailín"), ga, ga.Defaults, false, AccentSlip},
		{"ga fada exact", "cailín", e("cailín"), ga, ga.Defaults, false, Perfect},
		{"grc plain letters", "λογος", e("λόγος"), grc, grc.Defaults, false, Perfect},
		{"grc final sigma typed as sigma", "λογοσ", e("λόγος"), grc, grc.Defaults, false, Perfect},
		{"grc breathing reduced", "ανθρωπος", e("ἄνθρωπος"), grc, Rules{Accents: Ignore, Breathings: Reduced}, false, AccentSlip},
		{"grc full polytonic strict", "ἄνθρωπος", e("ἄνθρωπος"), grc, Rules{Accents: Strict, Breathings: Strict}, false, Perfect},
		{"grc oxia vs tonos", "λόγος", e("λόγος"), grc, Rules{Accents: Strict, Breathings: Strict}, false, Perfect},
		{"grc wrong accent strict", "λογός", e("λόγος"), grc, Rules{Accents: Strict, Breathings: Strict}, false, Miss},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Grade(tt.typed, tt.entry, tt.lang, tt.rules, tt.bs)
			if got.Tier != tt.want {
				t.Errorf("Grade(%q) = %v, want %v", tt.typed, got.Tier, tt.want)
			}
		})
	}
}

func TestAccentCycle(t *testing.T) {
	fr := lang(t, "fr")
	c, i, ok := fr.AccentCycle('é')
	if !ok || string(c) != "eéèêë" || i != 1 {
		t.Fatalf("got %q %d %v", string(c), i, ok)
	}
	c, i, ok = fr.AccentCycle('E')
	if !ok || string(c) != "EÉÈÊË" || i != 0 {
		t.Fatalf("got %q %d %v", string(c), i, ok)
	}
	if _, _, ok := fr.AccentCycle('z'); ok {
		t.Fatal("z should have no accents")
	}
}
