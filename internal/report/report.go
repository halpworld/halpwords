// Package report sends the pause menu's reports to Halpwords
// (halpwords-server's POST /api/v1/reports, its report.request schema).
//
// A report is "Something's wrong with the game" (a bug: the version,
// platform and seed, and the last crash.txt only if the player ticked the
// box after reading it) or "Something upset me" (what it was about, and
// optional text). Reports wait in a queue in the user's folder until the
// server has them, so they work offline. A game that isn't linked to an
// account sends no token, so its reports are anonymous; the report itself
// never holds the player's name or anything about the computer beyond its
// operating system.
//
// It has no Ebitengine imports.
package report

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Kinds of report.
const (
	KindBug   = "bug"   // Something's wrong with the game
	KindUpset = "upset" // Something upset me
)

// Abouts are what upset the player, in the order the game offers them.
var Abouts = []string{"name", "list", "picture", "player", "other"}

// AboutLabels are what the game shows for Abouts.
var AboutLabels = map[string]string{
	"name":    "A name",
	"list":    "A word list",
	"picture": "A picture",
	"player":  "A player",
	"other":   "Something else",
}

// Limits, as in the server's schema.
const (
	MaxText  = 500      // letters of text
	MaxField = 40       // letters of the version, platform and seed
	MaxCrash = 64 << 10 // bytes of crash.txt
)

// Game describes the game that sends a report.
type Game struct {
	Version  string `json:"version"`
	Platform string `json:"platform"`
}

// Report is one report, as sent (report.request.schema.json).
type Report struct {
	// ID is the game's own ID for the report, so the server stores a
	// report sent twice (after a lost answer) once.
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	About string `json:"about,omitempty"`
	Text  string `json:"text,omitempty"`
	Game  Game   `json:"game"`
	Seed  string `json:"seed,omitempty"`
	// Crash is the last crash.txt, in full, only when the player chose
	// to send it.
	Crash string `json:"crash,omitempty"`
}

// ErrInvalid means a report can't be sent as it is.
var ErrInvalid = errors.New("report: invalid")

// Bug makes a "Something's wrong with the game" report. crash is the
// crash.txt shown to the player; it goes in the report only if send is
// true, and is otherwise forgotten here.
func Bug(g Game, seed, text, crash string, send bool) Report {
	r := Report{ID: NewID(), Kind: KindBug, Text: cleanText(text), Game: tidyGame(g), Seed: cut(cleanLine(seed), MaxField)}
	if send {
		r.Crash = cleanCrash(crash)
	}
	return r
}

// Upset makes a "Something upset me" report about one of Abouts.
func Upset(g Game, about, text string) Report {
	return Report{ID: NewID(), Kind: KindUpset, About: about, Text: cleanText(text), Game: tidyGame(g)}
}

// Check reports whether r can be sent.
func (r Report) Check() error {
	switch {
	case r.ID == "" || len(r.ID) > 64:
		return ErrInvalid
	case r.Kind != KindBug && r.Kind != KindUpset:
		return ErrInvalid
	case r.Kind == KindUpset && !contains(Abouts, r.About):
		return ErrInvalid
	case r.Kind == KindBug && (r.About != ""):
		return ErrInvalid
	case r.Kind == KindUpset && (r.Seed != "" || r.Crash != ""):
		return ErrInvalid
	case utf8.RuneCountInString(r.Text) > MaxText || len(r.Crash) > MaxCrash:
		return ErrInvalid
	}
	for _, f := range []string{r.Game.Version, r.Game.Platform, r.Seed} {
		if utf8.RuneCountInString(f) > MaxField {
			return ErrInvalid
		}
	}
	return nil
}

// NewID returns a random UUID (version 4).
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // crypto/rand never fails on supported systems
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func tidyGame(g Game) Game {
	return Game{Version: cut(cleanLine(g.Version), MaxField), Platform: cut(cleanLine(g.Platform), MaxField)}
}

// cleanText keeps line breaks, drops other control characters, trims the
// ends and keeps at most MaxText letters.
func cleanText(s string) string {
	s = strings.ToValidUTF8(s, "")
	s = strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	return cut(strings.TrimSpace(s), MaxText)
}

func cleanLine(s string) string { return strings.Join(strings.Fields(cleanText(s)), " ") }

// cleanCrash keeps a crash report as written, valid UTF-8 without NULs,
// cut to MaxCrash bytes at a letter boundary.
func cleanCrash(s string) string {
	s = strings.ReplaceAll(strings.ToValidUTF8(s, "�"), "\x00", "")
	if strings.TrimSpace(s) == "" {
		return ""
	}
	if len(s) > MaxCrash {
		s = s[:MaxCrash]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return s
}

// cut keeps the first n letters of s.
func cut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
