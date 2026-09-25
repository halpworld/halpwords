// Package profile keeps what lasts between adventures: the player's
// settings, what they know of each word, and the Hall of Fame. It has no
// Ebitengine dependency.
package profile

import (
	"encoding/json"
	"errors"
	"io/fs"

	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/pkg/compete"
	"github.com/halpworld/halpwords/pkg/words"
)

// The files the profile is kept in.
const (
	settingsFile = "settings.json"
	memoryFile   = "progress.json"
	fameFile     = "halloffame.json"
)

// Timer is how much time battles give to type.
type Timer uint8

const (
	Normal Timer = iota
	Relaxed
	Fast
	numTimers
)

func (t Timer) String() string {
	return [...]string{"normal", "relaxed", "fast"}[t%numTimers]
}

// Scale is how much longer than normal the timers run.
func (t Timer) Scale() float64 {
	return [...]float64{1, 1.5, 0.75}[t%numTimers]
}

// Timers lists the timer settings in the order Settings offers them.
var Timers = []Timer{Relaxed, Normal, Fast}

// LangSettings are the settings for one language.
type LangSettings struct {
	Rules words.Rules
	// Highlight shows typing mistakes as they are made, like a Rune of
	// Clarity that never runs out.
	Highlight bool
	Timer     Timer
}

// Preset is the settings a language starts with, and the fixed rules of
// Hardcore runs: the language's grading rules, normal timers and no
// highlighting.
func Preset(lang *words.Language) LangSettings {
	return LangSettings{Rules: lang.Defaults}
}

// Settings are the player's settings: for the game, and for each
// language.
type Settings struct {
	Langs map[string]LangSettings `json:",omitempty"`
	// Game is nil until the game settings are first changed.
	Game *Options `json:",omitempty"`
}

// CRT is how much the screen looks like an old monitor.
type CRT uint8

const (
	CRTOff CRT = iota
	CRTSoft
	CRTStrong
	numCRT
)

func (c CRT) String() string { return [...]string{"off", "soft", "strong"}[c%numCRT] }

// MaxVolume is the loudest volume setting.
const MaxVolume = 10

// Options are the settings for the whole game: sound and screen.
type Options struct {
	Music   int // 0 to MaxVolume
	Effects int // 0 to MaxVolume
	CRT     CRT
	// Fullscreen is set when the game last ran full screen.
	Fullscreen bool
	// Shake shakes the view when the hero is hit. Some players find it
	// uncomfortable.
	Shake bool
}

// DefaultOptions are the game settings to start with.
func DefaultOptions() Options {
	return Options{Music: 6, Effects: 8, Shake: true}
}

// Options returns the game settings.
func (s *Settings) Options() Options {
	if s.Game == nil {
		return DefaultOptions()
	}
	o := *s.Game
	o.Music = max(0, min(MaxVolume, o.Music))
	o.Effects = max(0, min(MaxVolume, o.Effects))
	o.CRT %= numCRT
	return o
}

// SetOptions changes the game settings.
func (s *Settings) SetOptions(o Options) { s.Game = &o }

// For returns the settings for lang.
func (s *Settings) For(lang *words.Language) LangSettings {
	if ls, ok := s.Langs[lang.Code]; ok {
		return ls
	}
	return Preset(lang)
}

// Set changes the settings for lang.
func (s *Settings) Set(lang *words.Language, ls LangSettings) {
	if s.Langs == nil {
		s.Langs = map[string]LangSettings{}
	}
	s.Langs[lang.Code] = ls
}

// Profile is everything that lasts between adventures.
type Profile struct {
	Settings Settings
	// Memory is what the player knows of the words in each language, by
	// language code.
	Memory map[string]*words.Memory
	Fame   compete.HallOfFame
	// Name is the name last put in the Hall of Fame.
	Name string

	disk bool
}

// New returns an empty profile that is never written to disk.
func New() *Profile { return &Profile{Memory: map[string]*words.Memory{}} }

// Load reads the profile from the user's folder. Missing files are empty.
// A damaged file is kept to one side, with ".bad" after its name, and
// described in the errors; the rest of the profile still loads.
func Load() (*Profile, []error) {
	p := New()
	p.disk = true
	var errs []error
	read := func(name string, v any) {
		data, err := save.Read(name)
		if errors.Is(err, fs.ErrNotExist) {
			return
		}
		if err == nil {
			if err = json.Unmarshal(data, v); err == nil {
				return
			}
			save.Write(name+".bad", data)
		}
		errs = append(errs, err)
	}
	read(settingsFile, &p.Settings)
	var mem struct{ Memory map[string]*words.Memory }
	read(memoryFile, &mem)
	for code, m := range mem.Memory {
		if m != nil {
			if m.Cards == nil {
				m.Cards = map[string]*words.Card{}
			}
			p.Memory[code] = m
		}
	}
	var fame struct {
		Name string
		compete.HallOfFame
	}
	read(fameFile, &fame)
	p.Name, p.Fame = fame.Name, fame.HallOfFame
	return p, errs
}

// MemoryFor returns what the player knows of the words in language code.
func (p *Profile) MemoryFor(code string) *words.Memory {
	m := p.Memory[code]
	if m == nil {
		m = words.NewMemory()
		p.Memory[code] = m
	}
	return m
}

func (p *Profile) write(name string, v any) error {
	if !p.disk {
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return save.Write(name, data)
}

// SaveSettings writes the settings.
func (p *Profile) SaveSettings() error { return p.write(settingsFile, p.Settings) }

// SaveMemory writes what the player knows of their words.
func (p *Profile) SaveMemory() error {
	return p.write(memoryFile, struct{ Memory map[string]*words.Memory }{p.Memory})
}

// SaveFame writes the Hall of Fame.
func (p *Profile) SaveFame() error {
	return p.write(fameFile, struct {
		Name string
		compete.HallOfFame
	}{p.Name, p.Fame})
}
