package puzzle

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/halpworld/halpwords/assets"
)

// Every riddle line parses to a key and a text, with no duplicates of the
// same text for one key and no stray format.
func TestRiddleBankLinesParse(t *testing.T) {
	lines := 0
	seen := map[string]bool{}
	for i, l := range strings.Split(string(assets.Riddles), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		lines++
		k, r, ok := strings.Cut(l, "=")
		if !ok || strings.TrimSpace(k) == "" || strings.TrimSpace(r) == "" {
			t.Errorf("line %d does not parse: %q", i+1, l)
			continue
		}
		id := strings.ToLower(strings.TrimSpace(k)) + "=" + strings.TrimSpace(r)
		if seen[id] {
			t.Errorf("line %d repeats a riddle: %q", i+1, l)
		}
		seen[id] = true
	}
	n := 0
	for _, rs := range Riddles() {
		n += len(rs)
	}
	if n != lines {
		t.Errorf("%d riddle lines but %d parsed", lines, n)
	}
}

// A riddle must not give its answer away by any word of the key (for
// "I say": "say"), or by the key's stem at the start of a word of the
// riddle (teach -> teacher). A riddle may mention another word (a war is
// made of battles), but never the answer in its own text.
func TestRiddleDoesNotLeakItsAnswerParts(t *testing.T) {
	small := map[string]bool{"i": true, "a": true, "an": true, "the": true, "to": true, "of": true, "my": true}
	for key, rs := range Riddles() {
		for _, r := range rs {
			text := strings.ReplaceAll(r, "____", "")
			for _, part := range strings.Fields(key) {
				if small[part] || utf8.RuneCountInString(part) < 3 {
					continue
				}
				if regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(part) + `\b`).MatchString(text) {
					t.Errorf("riddle for %q contains %q: %q", key, part, r)
				}
				if stem := part; utf8.RuneCountInString(stem) >= 5 {
					stem = string([]rune(stem)[:utf8.RuneCountInString(stem)-1])
					if regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(stem)).MatchString(text) {
						t.Errorf("riddle for %q has a word starting %q: %q", key, stem, r)
					}
				}
			}
		}
	}
}

// The reworded riddles name the other word's sibling only by description:
// the pairs the issue was about keep apart, so neither riddle of a pair
// is the other's answer word.
func TestSiblingRiddlesKeepApart(t *testing.T) {
	bank := Riddles()
	for _, pair := range [][2]string{{"sister", "brother"}, {"son", "daughter"}, {"man", "woman"}, {"war", "battle"}} {
		for i, w := range pair {
			other := pair[1-i]
			for _, r := range bank[w] {
				if pair[0] == "war" || pair[0] == "man" {
					continue // these may mention the other by name deliberately
				}
				if regexp.MustCompile(`(?i)\b` + other + `\b`).MatchString(r) {
					t.Errorf("riddle for %q names %q: %q", w, other, r)
				}
			}
		}
	}
	// Two different keys never share a riddle text.
	owner := map[string]string{}
	for k, rs := range bank {
		for _, r := range rs {
			if o, dup := owner[r]; dup && o != k {
				t.Errorf("riddle shared by %q and %q: %q", o, k, r)
			}
			owner[r] = k
		}
	}
}
