package words

import (
	"reflect"
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
					t.Errorf("%s: %q graded %v against itself", l.File, a, r.Tier)
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
		{"en exact", "dog", e("dog"), English, English.Defaults, false, Perfect},
		{"en article", "the dog", e("dog"), English, English.Defaults, false, Perfect},
		{"en verb", "love", e("I love"), English, English.Defaults, false, Perfect},
		{"en to", "to love", e("I love"), English, English.Defaults, false, Perfect},
		{"en synonym", "gift", e("present", "gift"), English, English.Defaults, false, Perfect},
		{"en wrong", "cat", e("dog"), English, English.Defaults, false, Miss},
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

func TestParseImport(t *testing.T) {
	src := "title: Spreadsheet\ndog\tle chien\ncat = le chat | la chatte\n"
	l, err := ParseImport(strings.NewReader(src), "export.tsv")
	if err != nil {
		t.Fatal(err)
	}
	if l.Language != "" || len(l.Entries) != 2 || l.Entries[0].Answers[0] != "le chien" {
		t.Fatalf("got %+v", l)
	}
	l, err = ParseImport(strings.NewReader("language: French\ndog = le chien"), "fr.txt")
	if err != nil || l.Language != "fr" || l.Title != "fr" {
		t.Fatalf("got %+v, %v", l, err)
	}
	if _, err := Parse(strings.NewReader("language: fr\ndog\tle chien"), "t.txt"); err == nil {
		t.Error("Parse accepted a tab-separated word")
	}
	if _, err := ParseImport(strings.NewReader("language: xx\ndog = x"), "t.txt"); err == nil {
		t.Error("ParseImport accepted an unknown language")
	}
}

func TestFormatRoundTrip(t *testing.T) {
	src := "title: Test\nlanguage: la\n\n## a\nwater = aqua\n\nfire = ignis\n## b\nrose = rosa | rosae\n## a\nland = terra\n"
	l, err := Parse(strings.NewReader(src), "t.txt")
	if err != nil {
		t.Fatal(err)
	}
	l.Entries = append(l.Entries, Entry{Prompt: "war", Answers: []string{"bellum"}})
	back, err := Parse(strings.NewReader(string(l.Format())), "t.txt")
	if err != nil {
		t.Fatalf("%v in:\n%s", err, l.Format())
	}
	if back.Title != l.Title || back.Language != l.Language || len(back.Entries) != len(l.Entries) {
		t.Fatalf("got %+v", back)
	}
	tags := map[string]string{}
	for _, e := range back.Entries {
		tags[e.Prompt] = e.Tag + ":" + strings.Join(e.Answers, ",")
	}
	want := map[string]string{"water": "a:aqua", "land": "a:terra", "rose": "b:rosa,rosae", "war": ":bellum"}
	for p, w := range want {
		if tags[p] != w {
			t.Errorf("%s: got %q, want %q", p, tags[p], w)
		}
	}
}

func TestMerge(t *testing.T) {
	l := &List{Entries: []Entry{{Prompt: "dog", Answers: []string{"le chien"}}}}
	o := &List{Entries: []Entry{
		{Prompt: "Dog", Answers: []string{"Le chien", "un chien"}},
		{Prompt: "cat", Answers: []string{"le chat"}},
	}}
	if n := l.Merge(o); n != 1 {
		t.Errorf("added %d, want 1", n)
	}
	if len(l.Entries) != 2 || strings.Join(l.Entries[0].Answers, ",") != "le chien,un chien" {
		t.Errorf("got %+v", l.Entries)
	}
	if n := l.Merge(o); n != 0 {
		t.Errorf("merging again added %d", n)
	}
}

func TestFileName(t *testing.T) {
	for in, want := range map[string]string{
		"French - Animals": "french-animals.txt",
		"  Mé  Chéile!! ":  "me-cheile.txt",
		"Ἑλληνικά":         "words.txt",
	} {
		if got := FileName(in); got != want {
			t.Errorf("FileName(%q) = %q, want %q", in, got, want)
		}
	}
}

// Every starter list written by Format parses back to the same List.
func TestFormatStarterLists(t *testing.T) {
	lists, err := LoadFS(assets.Words, "words")
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lists {
		back, err := Parse(strings.NewReader(Format(l)), l.File)
		if err != nil {
			t.Fatalf("%s: %v", l.File, err)
		}
		if !reflect.DeepEqual(back, l) {
			t.Errorf("%s changed in a round trip:\n%s", l.File, Format(l))
		}
	}
}

const fullList = `# A list with every kind of line.
title: French - Unit 3: At home = chez moi
language: French
id: 01J9Z6ABCDEF
version: 7
level: A1
source: Dynamo 1, Unit 3 (Pearson)
licence: CC-BY-4.0
x-colour: blue
X-Later: anything: at all = here

house = la maison
## rooms
bedroom = la chambre
kitchen = la cuisine
friend = l'ami | l'amie
## sentences
>> Je dors dans ___. | la chambre
>> Voici ___ de Paul. | L’AMIE
##
yes = oui
## rooms
garden = le jardin
`

func TestHeadersAndCloze(t *testing.T) {
	l, err := Parse(strings.NewReader(fullList), "home.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := &List{
		Title: "French - Unit 3: At home = chez moi", Language: "fr",
		ID: "01J9Z6ABCDEF", Version: 7, Level: "A1",
		Source: "Dynamo 1, Unit 3 (Pearson)", Licence: "CC-BY-4.0",
		Entries: []Entry{
			{Prompt: "house", Answers: []string{"la maison"}},
			{Prompt: "bedroom", Answers: []string{"la chambre"}, Tag: "rooms"},
			{Prompt: "kitchen", Answers: []string{"la cuisine"}, Tag: "rooms"},
			{Prompt: "friend", Answers: []string{"l'ami", "l'amie"}, Tag: "rooms"},
			{Prompt: "yes", Answers: []string{"oui"}},
			{Prompt: "garden", Answers: []string{"le jardin"}, Tag: "rooms"},
		},
		Cloze: []ClozeLine{
			{Sentence: "Je dors dans ___.", Answer: "la chambre", Tag: "sentences"},
			{Sentence: "Voici ___ de Paul.", Answer: "L’AMIE", Tag: "sentences"},
		},
		File: "home.txt",
	}
	if !reflect.DeepEqual(l, want) {
		t.Fatalf("got  %+v\nwant %+v", l, want)
	}
	if a, ok := l.Cloze[1].Match(l.Entries[3]); !ok || a != "l'amie" {
		t.Errorf("Match = %q, %v", a, ok)
	}
	if l.Cloze[0].Fits(l.Entries[3]) {
		t.Error("la chambre fits friend")
	}
	back, err := Parse(strings.NewReader(Format(l)), "home.txt")
	if err != nil || !reflect.DeepEqual(back, l) {
		t.Fatalf("round trip: %v\n%s", err, Format(l))
	}
	// A list without the new lines is written as before.
	if got := Format(&List{Title: "T", Language: "la", Entries: []Entry{{Prompt: "water", Answers: []string{"aqua"}}}}); got != "title: T\nlanguage: la\n\nwater = aqua\n" {
		t.Errorf("Format wrote %q", got)
	}
}

func TestListErrors(t *testing.T) {
	for name, src := range map[string]string{
		"unknown key":        "language: fr\nlevle: A1\ndog = le chien",
		"bad version":        "language: fr\nversion: seven\ndog = le chien",
		"negative version":   "language: fr\nversion: -1\ndog = le chien",
		"cloze answer":       "language: fr\ndog = le chien\n>> Le ___ dort. | chat",
		"cloze without gap":  "language: fr\ndog = le chien\n>> Le chien dort. | le chien",
		"cloze two gaps":     "language: fr\ndog = le chien\n>> ___ et ___. | le chien",
		"cloze long gap":     "language: fr\ndog = le chien\n>> Le ____ dort. | le chien",
		"cloze no answer":    "language: fr\ndog = le chien\n>> Le ___ dort.",
		"cloze empty answer": "language: fr\ndog = le chien\n>> Le ___ dort. | ",
	} {
		_, err := Parse(strings.NewReader(src), "t.txt")
		if err == nil {
			t.Errorf("%s: no error", name)
			continue
		}
		if !strings.Contains(err.Error(), "line ") {
			t.Errorf("%s: %v does not name the line", name, err)
		}
	}
	_, err := Parse(strings.NewReader("language: fr\ndog = le chien\n\n>> Le ___ dort. | chat"), "t.txt")
	if err == nil || !strings.Contains(err.Error(), "t.txt line 4:") {
		t.Errorf("bad cloze answer: %v", err)
	}
}
