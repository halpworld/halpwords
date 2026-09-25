// Package safety holds Halpwords' family-safe policy for generated text:
// the instructions every AI request starts with, and the filter that
// throws away text unfit for a 13-year-old. The game and halpwords-server
// share it.
package safety

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Policy starts every request. It keeps what the model writes right for a
// 13-year-old, and it can't be turned off.
const Policy = `You write content for Halpwords, a friendly 8-bit dungeon-crawler game that helps secondary-school students (about 13 years old) learn to spell words in a foreign language.
Content rules, always:
- Family-safe and encouraging. Silly and spooky are fine; gore, cruelty, romance, alcohol, drugs, weapons detail, real people, politics, religion and anything scary for a child are not.
- No insults aimed at the player beyond gentle, playful monster boasting.
- Short, clear sentences suited to a learner.
- When asked for JSON, reply with the JSON only, no other text.`

// blocked are words that never belong in the game's generated text, in
// the game's languages. Generated text containing one is thrown away, so a
// few innocent sentences are lost to keep out the rest.
var blocked = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`
		fuck fucking fucker shit shitty bitch bastard asshole arse arsehole dick cock pussy cunt twat wanker
		slut whore porn porno sex sexy nude naked kill killing killed murder murderer suicide rape blood bloody
		gore drug drugs cocaine heroin beer wine vodka whisky alcohol drunk gun guns rifle bomb terrorist nazi
		hitler hate stupid idiot dumb retard
		merde putain connard connasse salope enculé bite chatte baiser tuer meurtre sang
		cac focáil`) {
		blocked[w] = true
	}
}

// Clean reports whether generated text is fit for the game: no blocked
// words, no links and no control characters.
func Clean(text string) bool {
	low := strings.ToLower(norm.NFC.String(text))
	if strings.Contains(low, "http") || strings.Contains(low, "www.") {
		return false
	}
	for _, r := range low {
		if unicode.IsControl(r) && r != '\n' {
			return false
		}
	}
	for _, w := range strings.FieldsFunc(low, func(r rune) bool { return !unicode.IsLetter(r) && r != '\'' }) {
		if blocked[strings.Trim(w, "'")] {
			return false
		}
	}
	return true
}
