package words

import "testing"

func TestQAArticleGrading(t *testing.T) {
	fr, la, grc, ga := lang(t, "fr"), lang(t, "la"), lang(t, "grc"), lang(t, "ga")
	e := func(a ...string) Entry { return Entry{Prompt: "x", Answers: a} }
	tests := []struct {
		name    string
		typed   string
		e       Entry
		lang    *Language
		rules   Rules
		tier    Tier
		artFlag bool
	}{
		{"fr la for le", "la chien", e("le chien"), fr, fr.Defaults, AccentSlip, true},
		{"fr les for la", "les maison", e("la maison"), fr, fr.Defaults, AccentSlip, true},
		{"fr le for les", "le chiens", e("les chiens"), fr, fr.Defaults, AccentSlip, true},
		{"fr l' for la", "l'école", e("la école"), fr, fr.Defaults, AccentSlip, true},
		{"fr la for l'", "la ami", e("l'ami"), fr, fr.Defaults, AccentSlip, true},
		{"fr une for un", "une chat", e("un chat"), fr, fr.Defaults, AccentSlip, true},
		{"fr same article perfect", "le chien", e("le chien"), fr, fr.Defaults, Perfect, false},
		{"fr no article perfect", "chien", e("le chien"), fr, fr.Defaults, Perfect, false},
		{"fr l' with word only", "ami", e("l'ami"), fr, fr.Defaults, Perfect, false},
		{"fr wrong article wrong noun stays Miss", "la chat", e("le chien"), fr, fr.Defaults, Miss, false},
		{"fr alt matches second answer, no flag", "la chienne", e("le chien", "la chienne"), fr, fr.Defaults, Perfect, false},
		{"fr wrong article + accent slip is slip", "la été", e("l'été"), fr, Rules{Accents: Reduced}, AccentSlip, true},
		{"fr required: no flag, ordinary graze", "la chien", e("le chien"), fr, Rules{Accents: Reduced, ArticlesRequired: true}, Graze, false},
		// Languages without articles never raise the flag.
		{"la", "puella", e("puella"), la, la.Defaults, Perfect, false},
		{"grc", "ὁ λόγος", e("λόγος"), grc, grc.Defaults, Miss, false},
		{"ga: no articles configured, no flag", "na cat", e("an cat"), ga, ga.Defaults, Graze, false},
		{"en a for the", "a dog", e("the dog"), English, English.Defaults, Perfect, false},
	}
	for _, tt := range tests {
		r := Grade(tt.typed, tt.e, tt.lang, tt.rules, false)
		if r.Tier != tt.tier || r.ArticleError != tt.artFlag {
			t.Errorf("%s: %+v, want tier %v article %v", tt.name, r, tt.tier, tt.artFlag)
		}
	}
}

// A wrong article plus a one-letter typo must not grade better than the typo
// alone (Graze).
func TestQAWrongArticleAndTypoNotBetterThanTypo(t *testing.T) {
	fr := lang(t, "fr")
	e := Entry{Prompt: "x", Answers: []string{"le chien"}}
	typo := Grade("le chiem", e, fr, fr.Defaults, false)
	both := Grade("la chiem", e, fr, fr.Defaults, false)
	if both.Tier > typo.Tier {
		t.Errorf("wrong article + typo graded %v, better than typo alone %v", both.Tier, typo.Tier)
	}
}

func TestQAGradeAmongAlternativesAndKeys(t *testing.T) {
	fr := lang(t, "fr")
	mk := func(p string, a ...string) Entry { return Entry{Prompt: p, Answers: a} }
	want := mk("father", "le père", "le papa")
	// typed answer is an alternative of another word -> Miss
	r := GradeAmong("la mère", want, []Entry{mk("mother", "la maman", "la mère")}, fr, fr.Defaults, false)
	if r.Tier != Miss || r.Confused != "la mère" {
		t.Errorf("alt of other: %+v", r)
	}
	// other word that shares an alternative is the same word
	r = GradeAmong("le pere", want, []Entry{mk("dad", "le papa", "le pere2")}, fr, fr.Defaults, false)
	if r.Tier != AccentSlip || r.Confused != "" {
		t.Errorf("shared alt: %+v", r)
	}
	// Perfect and Miss pass through untouched
	if r := GradeAmong("le père", want, []Entry{mk("m", "la mère")}, fr, fr.Defaults, false); r.Tier != Perfect {
		t.Errorf("perfect: %+v", r)
	}
	if r := GradeAmong("zzzzzz", want, []Entry{mk("m", "la mère")}, fr, fr.Defaults, false); r.Tier != Miss || r.Confused != "" {
		t.Errorf("miss: %+v", r)
	}
	// others with no answers are skipped without panic
	if r := GradeAmong("le pare", want, []Entry{{Prompt: "empty"}}, fr, fr.Defaults, false); r.Tier != Graze {
		t.Errorf("empty others: %+v", r)
	}
}

func TestQAClampBoxFromLoadedMemory(t *testing.T) {
	m := &Memory{Cards: map[string]*Card{
		"a = x": {Box: 99, Seen: 1}, "b = y": {Box: -5, Seen: 1}, "c = z": nil,
	}}
	m.Sanitize()
	if _, ok := m.Cards["c = z"]; ok {
		t.Error("nil card kept")
	}
	if m.Cards["a = x"].Box != Boxes || m.Cards["b = y"].Box != 0 {
		t.Errorf("boxes %d %d", m.Cards["a = x"].Box, m.Cards["b = y"].Box)
	}
	var nilm *Memory
	nilm.Sanitize()
	(&Memory{}).Sanitize()
	if ClampBox(Boxes+1) != Boxes || ClampBox(-1) != 0 || ClampBox(3) != 3 {
		t.Error("ClampBox")
	}
	// unsanitised memory must not panic anything that indexes by box
	raw := &Memory{Cards: map[string]*Card{"a = x": {Box: 99, Seen: 2}, "b = y": {Box: -4, Seen: 2, Misses: 1}}}
	es := []Entry{{Prompt: "a", Answers: []string{"x"}}, {Prompt: "b", Answers: []string{"y"}}}
	raw.Summarize(es)
	raw.Weakest(es, 5)
	raw.NewGrimoire(es, GrimoireWeakest)
	if raw.Box(es[0]) != Boxes || raw.Box(es[1]) != 0 {
		t.Error("Box not clamped")
	}
}
