package raid

import (
	"strings"
	"testing"

	"github.com/halpworld/halpwords/pkg/words"
)

// A raider can send, and the game grades, an answer of exactly the limit.
func TestQA3RaidGradesA60RuneAnswer(t *testing.T) {
	if MaxAnswer != 60 {
		t.Fatalf("MaxAnswer = %d", MaxAnswer)
	}
	for _, ans := range []string{
		strings.Repeat("a", MaxAnswer),
		strings.Repeat("é", MaxAnswer),
		strings.Repeat("αβ", MaxAnswer/2),
	} {
		e := words.Entry{Prompt: "p", Answers: []string{ans}}
		if words.TooLong(e) {
			t.Fatalf("%d-rune answer counted as too long", len([]rune(ans)))
		}
		lang := "fr"
		if strings.ContainsRune(ans, 'α') {
			lang = "grc"
		}
		if r := Grade(ans, e, lang, false); r.Tier != words.Perfect {
			t.Errorf("%q graded %v", ans, r.Tier)
		}
	}
}
