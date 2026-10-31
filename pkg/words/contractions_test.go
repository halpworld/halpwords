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
