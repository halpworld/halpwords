package llm

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

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
