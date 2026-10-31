package raid

import (
	"testing"

	"github.com/halpworld/halpwords/pkg/words"
)

var (
	mereE = words.Entry{Prompt: "mother", Answers: []string{"mère", "maman"}}
	pereE = words.Entry{Prompt: "father", Answers: []string{"père", "papa"}}
	merlE = words.Entry{Prompt: "blackbird", Answers: []string{"merle"}}
)

func TestGradeAmongEdges(t *testing.T) {
	all := []words.Entry{mereE, pereE, merlE}
	for _, tc := range []struct {
		name          string
		typed         string
		entry         words.Entry
		others        []words.Entry
		backspace     bool
		wantTier      words.Tier
		wantConfusion string
	}{
		{"other's answer, whole list", "père", mereE, all, false, words.Miss, "père"},
		{"entry is inside others, skipped itself", "mère", mereE, all, false, words.Perfect, ""},
		{"other's second alternative", "papa", mereE, all, false, words.Miss, ""}, // papa vs maman: not a graze anyway
		{"upper case", "PÈRE", mereE, all, false, words.Miss, "père"},
		{"accent slip on the other's answer", "pere", mereE, all, false, words.Miss, "père"},
		{"a real slip that is no one's answer", "mére", mereE, all, false, words.AccentSlip, ""},
		{"one letter off, not another's answer", "mare", mereE, all, false, words.Graze, ""},
		{"empty others", "père", mereE, []words.Entry{}, false, words.Graze, ""},
		{"nil others", "père", mereE, nil, false, words.Graze, ""},
		{"others only itself", "père", mereE, []words.Entry{mereE}, false, words.Graze, ""},
		{"duplicates of the other", "père", mereE, []words.Entry{pereE, pereE, pereE}, false, words.Miss, "père"},
		{"duplicates of the entry", "père", mereE, []words.Entry{mereE, mereE, pereE}, false, words.Miss, "père"},
		{"other with no answers", "père", mereE, []words.Entry{{Prompt: "x"}, pereE}, false, words.Miss, "père"},
		{"only an empty other", "père", mereE, []words.Entry{{Prompt: "x"}}, false, words.Graze, ""},
		{"a perfect answer with backspace", "mère", mereE, all, true, words.Correct, ""},
		{"a slip with backspace", "père", mereE, all, true, words.Miss, "père"},
		{"nothing typed", "", mereE, all, false, words.Miss, ""},
		{"a Miss stays a Miss", "chat", mereE, all, false, words.Miss, ""},
	} {
		got := GradeAmong(tc.typed, tc.entry, tc.others, "fr", tc.backspace)
		if got.Tier != tc.wantTier {
			t.Errorf("%s: tier %v, want %v (%+v)", tc.name, got.Tier, tc.wantTier, got)
		}
		if tc.wantConfusion != "" && got.Confused != tc.wantConfusion {
			t.Errorf("%s: confused %q, want %q", tc.name, got.Confused, tc.wantConfusion)
		}
		if tc.wantConfusion == "" && got.Confused != "" {
			t.Errorf("%s: confused %q, want none", tc.name, got.Confused)
		}
	}
}

// An entry that shares an answer with another (a synonym) isn't "another
// word": typing the shared answer is right, and a slip toward the
// shared answer stays a Graze.
func TestGradeAmongSynonymsAreNotOtherWords(t *testing.T) {
	a := words.Entry{Prompt: "mum", Answers: []string{"maman", "mère"}}
	b := words.Entry{Prompt: "mother", Answers: []string{"mère"}}
	if r := GradeAmong("mère", b, []words.Entry{a, b}, "fr", false); r.Tier != words.Perfect {
		t.Errorf("shared answer: %+v", r)
	}
	if r := GradeAmong("mere", b, []words.Entry{a}, "fr", false); r.Tier != words.AccentSlip {
		t.Errorf("accent slip toward the shared answer: %+v", r)
	}
	if r := GradeAmong("mèrs", b, []words.Entry{a}, "fr", false); r.Tier != words.Graze {
		t.Errorf("slip with a synonym around: %+v", r)
	}
}

// Unknown language code, the languages with other rules, and equality with
// plain Grade whenever no other word is confused.
func TestGradeAmongLanguagesAndPlainGrade(t *testing.T) {
	e := words.Entry{Prompt: "dog", Answers: []string{"madra"}}
	o := words.Entry{Prompt: "cat", Answers: []string{"cat"}}
	for _, code := range []string{"ga", "fr", "la", "grc", "xx", ""} {
		for _, typed := range []string{"madra", "madre", "Madra", "", "cat", "madrá"} {
			g := GradeAmong(typed, e, []words.Entry{o}, code, false)
			p := Grade(typed, e, code, false)
			if g.Tier != p.Tier && !(g.Tier == words.Miss && p.Tier == words.Graze) {
				t.Errorf("%q lang %q: GradeAmong %v vs Grade %v", typed, code, g.Tier, p.Tier)
			}
		}
	}
	grc := words.Entry{Prompt: "word", Answers: []string{"λόγος"}}
	oth := words.Entry{Prompt: "reason", Answers: []string{"λόγοι"}}
	if r := GradeAmong("λόγοι", grc, []words.Entry{oth}, "grc", false); r.Tier != words.Miss || r.Confused != "λόγοι" {
		t.Errorf("greek: %+v", r)
	}
	// A large "others" set stays quick and right.
	big := make([]words.Entry, 2000)
	for i := range big {
		big[i] = words.Entry{Prompt: "w", Answers: []string{"zzzz"}}
	}
	big = append(big, pereE)
	if r := GradeAmong("père", mereE, big, "fr", false); r.Tier != words.Miss {
		t.Errorf("large others: %+v", r)
	}
}
