package llm

import (
	"encoding/json"
	"errors"
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

// decodeJSON reads the first JSON value in a model's reply into v. Models
// sometimes wrap JSON in a code fence or add a sentence around it.
func decodeJSON(text string, v any) error {
	err := error(ErrEmpty)
	for tries := 0; tries < 8; tries++ {
		start := strings.IndexByte(text, '{')
		if start < 0 {
			break
		}
		text = text[start:]
		if err = json.NewDecoder(strings.NewReader(text)).Decode(v); err == nil {
			return nil
		}
		text = text[1:]
	}
	return errors.Join(ErrEmpty, err)
}

// short reports whether s is non-empty and at most n characters long.
func short(s string, n int) bool {
	c := len([]rune(s))
	return c > 0 && c <= n
}

// drawable are the characters the game's font has (see tools/fontsubset).
var drawable = [][2]rune{
	{0x0020, 0x007E}, {0x00A0, 0x024F}, {0x0370, 0x03FF}, {0x1E00, 0x1EFF},
	{0x1F00, 0x1FFF}, {0x2000, 0x206F}, {0x20A0, 0x20CF}, {0x2190, 0x21FF}, {0x2500, 0x27BF},
}

// tidy makes generated text ready to draw: composed accents, single spaces,
// and nothing the game's font can't draw, such as emoji.
func tidy(s string) string {
	s = norm.NFC.String(s)
	var b strings.Builder
	for _, r := range s {
		if unicode.IsSpace(r) {
			b.WriteByte(' ')
			continue
		}
		for _, rg := range drawable {
			if r >= rg[0] && r <= rg[1] {
				b.WriteRune(r)
				break
			}
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
