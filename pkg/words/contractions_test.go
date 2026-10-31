package words

import "testing"

func TestEnglishContractions(t *testing.T) {
	cases := []struct {
		typed, answer string
		want          Tier
	}{
		{"I'm", "I am", Perfect},
		{"I am", "I'm", Perfect},
		{"i’m", "I am", Perfect},
		{"am", "I am", Perfect},
		{"don't", "do not", Perfect},
		{"do not", "don't", Perfect},
		{"it's", "it is", Perfect},
		{"cannot", "can't", Perfect},
		{"won't", "will not", Perfect},
		{"I'm", "I was", Miss},
		{"don't", "do", Miss},
		{"dont", "don't", Graze},
		{"cant", "can't", Graze},
		{"its", "it's", Graze},
		{"lets", "let's", Graze},
		{"didnt", "didn't", Graze},
		{"wont", "won't", Graze},
		{"isnt", "isn't", Graze},
		{"youre", "you're", Graze},
	}
	for _, c := range cases {
		e := Entry{Prompt: "x", Answers: []string{c.answer}}
		got := Grade(c.typed, e, English, English.Defaults, false)
		if got.Tier != c.want {
			t.Errorf("Grade(%q, %q) = %v, want %v", c.typed, c.answer, got.Tier, c.want)
		}
	}
}

func TestContractionsOnlyForEnglish(t *testing.T) {
	fr := lang(t, "fr")
	e := Entry{Prompt: "x", Answers: []string{"j'ai"}}
	if got := Grade("j'ai", e, fr, fr.Defaults, false); got.Tier != Perfect {
		t.Errorf("French j'ai: %v", got.Tier)
	}
}

func TestContractionsCaseSensitive(t *testing.T) {
	r := English.Defaults
	r.CaseSensitive = true
	e := Entry{Prompt: "x", Answers: []string{"I'm"}}
	if got := Grade("i'm", e, English, r, false); got.Tier != Miss {
		t.Errorf("case-sensitive i'm for I'm: %v, want Miss", got.Tier)
	}
	if got := Grade("I'm", e, English, r, false); got.Tier != Perfect {
		t.Errorf("case-sensitive I'm: %v, want Perfect", got.Tier)
	}
}
