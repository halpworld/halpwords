package words

import "testing"

func gradeEN(typed string, answers ...string) Tier {
	e := Entry{Prompt: "x", Answers: answers}
	return Grade(typed, e, English, English.Defaults, false).Tier
}

// Apostrophe variants all count as the straight one.
func TestContractionApostropheVariants(t *testing.T) {
	for _, ap := range []string{"'", "’", "‘", "ʼ", "`", "´"} {
		if got := gradeEN("don"+ap+"t", "do not"); got != Perfect {
			t.Errorf("apostrophe %q typed: %v", ap, got)
		}
		if got := gradeEN("do not", "don"+ap+"t"); got != Perfect {
			t.Errorf("apostrophe %q in the answer: %v", ap, got)
		}
		if got := gradeEN("I"+ap+"M", "I am"); got != Perfect {
			t.Errorf("apostrophe %q upper case: %v", ap, got)
		}
	}
}

// Words with an apostrophe that are not contractions are left alone: they
// still need to be typed as they are, with or without the apostrophe
// variants, and are not made equal to anything else.
func TestNonContractionApostrophes(t *testing.T) {
	for _, tc := range []struct {
		typed, answer string
		want          Tier
	}{
		{"o'clock", "o'clock", Perfect},
		{"o’clock", "o'clock", Perfect},
		{"rock 'n' roll", "rock 'n' roll", Perfect},
		{"rock ’n’ roll", "rock 'n' roll", Perfect},
		{"O'Brien", "O'Brien", Perfect},
		{"o'brien", "O'Brien", Perfect},
		{"John's book", "John's book", Perfect},
		{"John is book", "John's book", Miss}, // a possessive is not "is"
		{"the dog's bone", "the dog's bone", Perfect},
		{"dogs bone", "dog's bone", Graze}, // one letter off, as before
		{"it's", "its", Miss}, // "its" is not "it is"
		{"its", "it's", Miss},
		{"y'all", "you all", Miss}, // not in the table
		{"ma'am", "ma'am", Perfect},
	} {
		if got := gradeEN(tc.typed, tc.answer); got != tc.want {
			t.Errorf("Grade(%q, %q) = %v, want %v", tc.typed, tc.answer, got, tc.want)
		}
	}
}

// can't, cannot and can not are the same; cant (no apostrophe) is not.
func TestCantCannotCanNot(t *testing.T) {
	forms := []string{"can't", "cannot", "can not", "CAN'T", "Cannot", "can’t"}
	for _, a := range forms {
		for _, b := range forms {
			if got := gradeEN(a, b); got != Perfect {
				t.Errorf("Grade(%q, %q) = %v", a, b, got)
			}
		}
	}
	if got := gradeEN("cant", "can't"); got == Perfect {
		t.Errorf("cant = can't: %v", got)
	}
	if got := gradeEN("can", "can't"); got != Miss {
		t.Errorf("can = can't: %v", got)
	}
	if got := gradeEN("cannot", "can"); got != Miss {
		t.Errorf("cannot = can: %v", got)
	}
}

// Contractions inside longer phrases, either side, with a repeated and an
// unrelated one.
func TestContractionsInPhrases(t *testing.T) {
	for _, tc := range []struct {
		typed, answer string
		want          Tier
	}{
		{"I don't know", "I do not know", Perfect},
		{"I do not know", "I don't know", Perfect},
		{"they're sure it's here", "they are sure it is here", Perfect},
		{"they are sure it's here", "they're sure it is here", Perfect},
		{"I don't know", "I do know", Miss},
		{"I'm not sure", "I am not sure", Perfect},
		{"I'm not sure", "I'm not here", Miss},
		{"let's go", "let us go", Perfect},
		{"let us go", "let's go", Perfect},
		{"what's up", "what is up", Perfect},
		{"I'll go", "I will go", Perfect},
		{"I’d go", "I would go", Perfect},
		{"  don't   go  ", "do not go", Perfect},
	} {
		if got := gradeEN(tc.typed, tc.answer); got != tc.want {
			t.Errorf("Grade(%q, %q) = %v, want %v", tc.typed, tc.answer, got, tc.want)
		}
	}
}

// Case rules: insensitive by default; a case-sensitive list still sees
// differences in what isn't a contraction. (A contraction expands in lower
// case, so a case-sensitive list is only checked not to panic or to match
// something wrong.)
func TestContractionsCaseAndRules(t *testing.T) {
	if got := gradeEN("DON'T", "Do Not"); got != Perfect {
		t.Errorf("upper vs title case: %v", got)
	}
	cs := English.Defaults
	cs.CaseSensitive = true
	e := Entry{Prompt: "x", Answers: []string{"Paris"}}
	if got := Grade("paris", e, English, cs, false).Tier; got == Perfect {
		t.Errorf("case-sensitive list accepted paris: %v", got)
	}
	e = Entry{Prompt: "x", Answers: []string{"don't"}}
	if got := Grade("don't", e, English, cs, false).Tier; got != Perfect {
		t.Errorf("case-sensitive identical contraction: %v", got)
	}
	if got := Grade("do not", e, English, cs, false).Tier; got != Perfect {
		t.Errorf("case-sensitive long form: %v", got)
	}
	// Backspace still downgrades.
	if got := Grade("don't", Entry{Prompt: "x", Answers: []string{"do not"}}, English, English.Defaults, true).Tier; got != Correct {
		t.Errorf("with backspace: %v", got)
	}
}

// Other languages are untouched: French elision, Irish h- and t- prefixes,
// an English-looking contraction in a French list, and a nil language.
func TestOtherLanguagesUnaffectedByContractions(t *testing.T) {
	fr, ga := lang(t, "fr"), lang(t, "ga")
	for _, tc := range []struct {
		l             *Language
		typed, answer string
		want          Tier
	}{
		{fr, "l'ami", "l'ami", Perfect},
		{fr, "l’ami", "l'ami", Perfect},
		{fr, "d'accord", "d'accord", Perfect},
		{fr, "d'accord", "de accord", Miss},
		{fr, "j'ai", "je ai", Miss},
		{fr, "c'est", "ce est", Miss},
		{fr, "n'est", "ne est", Miss},
		{fr, "it's", "it is", Miss},
		{fr, "don't", "do not", Miss},
		{ga, "an t-ainm", "an t-ainm", Perfect},
		{ga, "an t-ainm", "an tainm", Graze}, // one letter off, never Perfect
		{ga, "h-ainm", "h-ainm", Perfect},
		{ga, "d'fhág", "d'fhág", Perfect},
		{ga, "it's", "it is", Miss},
	} {
		e := Entry{Prompt: "x", Answers: []string{tc.answer}}
		if got := Grade(tc.typed, e, tc.l, tc.l.Defaults, false).Tier; got != tc.want {
			t.Errorf("%s Grade(%q, %q) = %v, want %v", tc.l.Code, tc.typed, tc.answer, got, tc.want)
		}
	}
	// A nil language never expands.
	e := Entry{Prompt: "x", Answers: []string{"do not"}}
	if got := Grade("don't", e, nil, English.Defaults, false).Tier; got == Perfect {
		t.Errorf("nil language expanded contractions: %v", got)
	}
}

// Accepted answers listed both ways, and GradeAmong's confusions, are not
// disturbed: the best of several answers wins.
func TestContractionsSeveralAnswers(t *testing.T) {
	e := Entry{Prompt: "x", Answers: []string{"cannot", "I am unable"}}
	for _, typed := range []string{"can't", "can not", "I am unable", "i’m unable"} {
		if got := Grade(typed, e, English, English.Defaults, false).Tier; got != Perfect {
			t.Errorf("%q: %v", typed, got)
		}
	}
}

func FuzzExpandContractions(f *testing.F) {
	for _, s := range []string{"don't", "", "'", "''", "can'tcan't", "I'm  you're", "cannot'", "\x00'", "ǅ'm"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		a := expandContractions(s)
		// Idempotent on its own output.
		if b := expandContractions(a); b != a {
			t.Errorf("not idempotent: %q -> %q -> %q", s, a, b)
		}
		e := Entry{Prompt: "x", Answers: []string{s, "do not"}}
		_ = Grade(s, e, English, English.Defaults, false)
	})
}
