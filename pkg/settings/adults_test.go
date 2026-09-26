package settings

import (
	"testing"

	"github.com/halpworld/halpwords/pkg/words"
)

func ptr[T any](v T) *T { return &v }

// TestPrecedence: a learner's accommodation beats an assignment, which
// beats a class, which beats the child's own choice, whatever order the
// layers come in.
func TestPrecedence(t *testing.T) {
	fr, _ := words.Lookup("fr")
	own := Lang{Rules: words.Rules{Accents: words.Reduced}, Timer: Fast}
	class := Layer{Source: Class, Role: "teacher", Accents: ptr(words.Strict), Timer: ptr(Normal)}
	assign := Layer{Source: Assignment, Role: "teacher", Accents: ptr(words.Reduced)}
	acc := Layer{Source: Accommodation, Role: "guardian", Accents: ptr(words.Ignore), Timer: ptr(Relaxed)}

	for _, c := range []struct {
		name    string
		layers  []Layer
		accents words.Strictness
		timer   Timer
		by      Decided
	}{
		{"own", nil, words.Reduced, Fast, Decided{Decider{Source: Own}, Decider{Source: Own}}},
		{"class", []Layer{class}, words.Strict, Normal,
			Decided{Decider{Class, "teacher"}, Decider{Class, "teacher"}}},
		{"assignment over class", []Layer{assign, class}, words.Reduced, Normal,
			Decided{Decider{Assignment, "teacher"}, Decider{Class, "teacher"}}},
		{"accommodation over all", []Layer{acc, assign, class}, words.Ignore, Relaxed,
			Decided{Decider{Accommodation, "guardian"}, Decider{Accommodation, "guardian"}}},
		{"accommodation first or last", []Layer{class, assign, acc}, words.Ignore, Relaxed,
			Decided{Decider{Accommodation, "guardian"}, Decider{Accommodation, "guardian"}}},
	} {
		got, by := Resolve(fr, own, Decided{}, c.layers...)
		if got.Rules.Accents != c.accents || got.Timer != c.timer || by != c.by {
			t.Errorf("%s: accents %v, timer %v, by %+v; want %v, %v, %+v", c.name, got.Rules.Accents, got.Timer, by,
				c.accents, c.timer, c.by)
		}
	}
}

// TestStrictestAtTheSameLevel: two classes, or a class and the learner's
// settings on the website, give the stricter of their settings.
func TestStrictestAtTheSameLevel(t *testing.T) {
	fr, _ := words.Lookup("fr")
	base := Lang{Rules: words.Rules{Accents: words.Ignore}, Timer: Relaxed}
	by := Decided{Decider{Learner, "guardian"}, Decider{Learner, "guardian"}}
	for _, order := range [][]Layer{
		{{Source: Class, Role: "teacher", Accents: ptr(words.Reduced), Timer: ptr(Fast)},
			{Source: Class, Role: "teacher", Accents: ptr(words.Strict), Timer: ptr(Normal)}},
		{{Source: Class, Role: "teacher", Accents: ptr(words.Strict), Timer: ptr(Normal)},
			{Source: Class, Role: "teacher", Accents: ptr(words.Reduced), Timer: ptr(Fast)}},
	} {
		got, d := Resolve(fr, base, by, order...)
		if got.Rules.Accents != words.Strict || got.Timer != Fast || d.Accents.Source != Class || d.Timer.Source != Class {
			t.Errorf("got %+v by %+v", got, d)
		}
	}
	// A class can't make the learner's settings more lenient.
	strict := Lang{Rules: words.Rules{Accents: words.Strict}, Timer: Fast}
	got, d := Resolve(fr, strict, by, Layer{Source: Class, Role: "teacher", Accents: ptr(words.Ignore), Timer: ptr(Relaxed)})
	if got.Rules.Accents != words.Strict || got.Timer != Fast || d != by {
		t.Errorf("a lenient class preset: %+v by %+v", got, d)
	}
	if roles := d.Roles(); len(roles) != 1 || roles[0] != "guardian" {
		t.Errorf("roles %v", roles)
	}
}

// TestGreekBreathings: in Ancient Greek, accents set breathings too.
func TestGreekBreathings(t *testing.T) {
	grc, ok := words.Lookup("grc")
	if !ok {
		t.Skip("no Ancient Greek")
	}
	got, _ := Resolve(grc, Preset(grc), Decided{}, Layer{Source: Accommodation, Accents: ptr(words.Ignore)})
	if got.Rules.Accents != words.Ignore || got.Rules.Breathings != words.Ignore {
		t.Errorf("%+v", got.Rules)
	}
}
