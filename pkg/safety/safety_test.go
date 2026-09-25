package safety

import (
	"strings"
	"testing"
)

func TestClean(t *testing.T) {
	for text, want := range map[string]bool{
		"The goblin guards the door.":    true,
		"Le gobelin garde la porte.":     true,
		"Don't be silly, adventurer!":    true,
		"What a stupid hero.":            false,
		"Visit http://example.com today": false,
		"see www.example.com":            false,
		"a line\x07with a bell":          false,
		"two\nlines are fine":            true,
		"He drinks WINE.":                false,
		"Sussex is a place, not a word":  true,
		"'idiot' in quotes is still out": false,
		"Cá bhfuil an leabharlann?":      true,
		"":                               true,
	} {
		if got := Clean(text); got != want {
			t.Errorf("Clean(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestPolicy(t *testing.T) {
	if !strings.Contains(Policy, "Family-safe") {
		t.Error("Policy lost its family-safe rule")
	}
}
