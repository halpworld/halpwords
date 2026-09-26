// Package settings holds a player's settings for one language: how
// answers are graded, typing highlights and the battle timer. The game
// keeps them in the player's profile (settings.json), and a grown-up can
// set them on halpwords-server for a linked game, which then shows them
// locked. Both sides use these types, so they agree on the values and
// the JSON.
package settings

import "github.com/halpworld/halpwords/pkg/words"

// Timer is how much time battles give to type. The values are stored in
// saves and sent by the server: Normal 0, Relaxed 1, Fast 2.
type Timer uint8

// The timer settings.
const (
	Normal Timer = iota
	Relaxed
	Fast
	numTimers
)

// String is the timer's name: "normal", "relaxed" or "fast". An unknown
// value is named as the timer it plays as (see Scale).
func (t Timer) String() string {
	return [...]string{"normal", "relaxed", "fast"}[t%numTimers]
}

// Scale is how much longer than normal the timers run. An unknown value
// wraps around (3 plays as Normal).
func (t Timer) Scale() float64 {
	return [...]float64{1, 1.5, 0.75}[t%numTimers]
}

// Valid reports whether t is Normal, Relaxed or Fast.
func (t Timer) Valid() bool { return t < numTimers }

// Timers lists the timer settings in the order the game's Settings
// offers them. Don't change it.
var Timers = []Timer{Relaxed, Normal, Fast}

// ParseTimer returns the timer named s ("relaxed").
func ParseTimer(s string) (Timer, bool) {
	for _, t := range Timers {
		if t.String() == s {
			return t, true
		}
	}
	return Normal, false
}

// Lang are the settings for one language. Its JSON is
// {"Rules":{"Accents":1,"Breathings":0,"ArticlesRequired":false,
// "CaseSensitive":false},"Highlight":false,"Timer":0}, as the game has
// always kept it in settings.json.
type Lang struct {
	Rules words.Rules
	// Highlight shows typing mistakes as they are made, like a Rune of
	// Clarity that never runs out.
	Highlight bool
	Timer     Timer
}

// Preset is the settings a language starts with, and the fixed rules of
// Hardcore runs: the language's grading rules, normal timers and no
// highlighting.
func Preset(lang *words.Language) Lang {
	return Lang{Rules: lang.Defaults}
}
