package scene

import (
	"strings"
	"testing"

	"github.com/halpworld/halpwords/pkg/words"
)

func TestMistakeLineArticle(t *testing.T) {
	fr, _ := words.Lookup("fr")
	e := words.Entry{Prompt: "dog", Answers: []string{"le chien"}}
	res := words.Grade("la chien", e, fr, fr.Defaults, false)
	if !res.ArticleError {
		t.Fatal("not an article error")
	}
	l := mistakeLine("You typed la chien.", "la chien", res, fr)
	if !strings.Contains(l.text, "Check the article: le chien.") || strings.Contains(l.text, "letter") {
		t.Fatalf("%q", l.text)
	}
}

func TestMistakeLineConfusedFits(t *testing.T) {
	res := words.Result{Tier: words.Miss, Confused: strings.Repeat("très long ", 20)}
	l := mistakeLine("You typed x.", "x", res, nil)
	if n := len([]rune(l.text)); n > 70 || !strings.Contains(l.text, "another word in this list") {
		t.Fatalf("%d %q", n, l.text)
	}
}
