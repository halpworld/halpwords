package audio

import "math"

// ID names a sound effect.
type ID int

const (
	// Menus.
	Blip   ID = iota // moving through a menu
	Select           // choosing a menu item
	Back             // leaving a screen

	// Typing.
	Key    // a letter typed
	Erase  // Backspace
	Accent // Tab changed an accent
	Warn   // the dodge timer is running out

	// Grades, for spelling practice.
	Perfect
	Correct
	Slip
	Graze
	Wrong

	// Battles.
	Alert   // a battle starts
	Hit     // an attack lands
	Crit    // a critical hit
	Weak    // a graze or accent slip lands softly
	Fumble  // the hero misses and nicks themself
	Clang   // armor turns an attack aside
	Hurt    // the hero is hit
	Dodge   // the hero dodges
	Defeat  // a monster is defeated
	LevelUp // the hero gains a level
	Potion  // the hero drinks a potion
	Flee    // the hero escapes
	Fall    // the hero is defeated

	// Exploring.
	Step   // a footstep
	Bump   // walking into a wall
	Door   // a door creaks open
	Unseal // the runes on a sealed door break
	Chest  // a chest opens
	Zap    // a trap or rune stings the hero
	Stairs // going down the stairs
	Chomp  // a mimic chest wakes up

	Count // the number of sounds
)

// names are for tests and debugging.
var names = [Count]string{
	"blip", "select", "back",
	"key", "erase", "accent", "warn",
	"perfect", "correct", "slip", "graze", "wrong",
	"alert", "hit", "crit", "weak", "fumble", "clang", "hurt", "dodge",
	"defeat", "levelup", "potion", "flee", "fall",
	"step", "bump", "door", "unseal", "chest", "zap", "stairs", "chomp",
}

func (id ID) String() string {
	if id < 0 || id >= Count {
		return "unknown"
	}
	return names[id]
}

// note returns the pitch k semitones above A4 (440 Hz).
func note(k float64) float64 { return 440 * math.Exp2(k/12) }

// Semitones above A4 for the notes the jingles use.
const (
	nC4 = -9
	nE4 = -5
	nG4 = -2
	nA4 = 0
	nC5 = 3
	nD5 = 5
	nE5 = 7
	nG5 = 10
	nA5 = 12
	nC6 = 15
	nE6 = 19
	nG6 = 22
	nC7 = 27
)

// beep is a short square-wave note at semitone k, starting at delay.
func beep(k, delay, hold, vol float64) Tone {
	return Tone{Wave: Square, Freq: note(k), Duty: 0.25, Delay: delay, Attack: 0.004, Hold: hold, Release: 0.06, Volume: vol}
}

// arpeggio plays notes one after another, gap seconds apart.
func arpeggio(wave Wave, gap, hold, vol float64, keys ...float64) Sound {
	s := make(Sound, len(keys))
	for i, k := range keys {
		s[i] = Tone{Wave: wave, Freq: note(k), Duty: 0.25, Delay: float64(i) * gap, Attack: 0.004, Hold: hold, Release: 0.08, Volume: vol}
	}
	return s
}

// Sounds holds every sound effect, indexed by ID.
var Sounds = [Count]Sound{
	Blip:   {beep(nE6, 0, 0.012, 0.35)},
	Select: {beep(nC6, 0, 0.03, 0.4), beep(nG6, 0.05, 0.05, 0.4)},
	Back:   {beep(nG5, 0, 0.03, 0.35), beep(nC5, 0.05, 0.04, 0.35)},

	Key:    {{Wave: Square, Freq: 1400, Slide: -3, Duty: 0.5, Attack: 0.001, Hold: 0.006, Release: 0.02, Volume: 0.22}},
	Erase:  {{Wave: Square, Freq: 700, Slide: -4, Duty: 0.5, Attack: 0.001, Hold: 0.01, Release: 0.03, Volume: 0.22}},
	Accent: {{Wave: Triangle, Freq: note(nE6), Slide: 2, Attack: 0.002, Hold: 0.02, Release: 0.05, Volume: 0.5}},
	Warn:   {beep(nA5, 0, 0.04, 0.5), beep(nA5, 0.12, 0.04, 0.5)},

	Perfect: arpeggio(Square, 0.07, 0.04, 0.55, nC6, nE6, nG6, nC7),
	Correct: arpeggio(Square, 0.07, 0.04, 0.5, nC6, nG6),
	Slip:    arpeggio(Triangle, 0.09, 0.06, 0.35, nE5, nD5),
	Graze:   arpeggio(Triangle, 0.1, 0.06, 0.35, nC5, nG4),
	Wrong: {
		{Wave: Saw, Freq: 150, Slide: -0.5, Attack: 0.005, Hold: 0.18, Release: 0.1, Volume: 0.45},
		{Wave: Saw, Freq: 155, Slide: -0.5, Attack: 0.005, Hold: 0.18, Release: 0.1, Volume: 0.45},
	},

	Alert: {beep(nA4, 0, 0.05, 0.4), beep(nE5, 0.08, 0.08, 0.4)},
	Hit: {
		{Wave: Noise, Freq: 3000, Slide: -3, Attack: 0.002, Hold: 0.03, Release: 0.12, Volume: 0.7},
		{Wave: Square, Freq: 330, Slide: -3, Duty: 0.5, Attack: 0.002, Hold: 0.02, Release: 0.08, Volume: 0.4},
	},
	Crit: {
		{Wave: Noise, Freq: 4000, Slide: -2, Attack: 0.002, Hold: 0.05, Release: 0.2, Volume: 0.8},
		{Wave: Square, Freq: 520, Slide: -2, Duty: 0.5, Attack: 0.002, Hold: 0.03, Release: 0.1, Volume: 0.4},
		beep(nC6, 0.08, 0.04, 0.35), beep(nG6, 0.14, 0.08, 0.35),
	},
	Weak: {
		{Wave: Noise, Freq: 1500, Slide: -3, Attack: 0.002, Hold: 0.02, Release: 0.08, Volume: 0.45},
	},
	Fumble: {
		{Wave: Triangle, Freq: 400, Slide: -2.5, Attack: 0.003, Hold: 0.08, Release: 0.15, Volume: 0.45},
	},
	Clang: {
		{Wave: Square, Freq: 1800, Duty: 0.3, Vibrato: 0.3, VibratoHz: 30, Attack: 0.001, Hold: 0.02, Release: 0.3, Volume: 0.3},
		{Wave: Square, Freq: 2710, Duty: 0.4, Attack: 0.001, Hold: 0.01, Release: 0.22, Volume: 0.2},
		{Wave: Noise, Freq: 6000, Attack: 0.001, Hold: 0.01, Release: 0.05, Volume: 0.4},
	},
	Hurt: {
		{Wave: Noise, Freq: 900, Slide: -2, Attack: 0.002, Hold: 0.06, Release: 0.15, Volume: 0.8},
		{Wave: Square, Freq: 220, Slide: -2, Duty: 0.5, Attack: 0.002, Hold: 0.05, Release: 0.12, Volume: 0.45},
	},
	Dodge: {
		{Wave: Noise, Freq: 800, Slide: 3, Attack: 0.04, Hold: 0.02, Release: 0.12, Volume: 0.45},
		beep(nE6, 0.1, 0.03, 0.25),
	},
	Defeat: {
		{Wave: Square, Freq: 700, Slide: -2.5, Duty: 0.5, Vibrato: 1.5, VibratoHz: 18, Attack: 0.005, Hold: 0.25, Release: 0.2, Volume: 0.35},
		{Wave: Noise, Freq: 2000, Slide: -2, Attack: 0.01, Hold: 0.15, Release: 0.3, Volume: 0.4},
	},
	LevelUp: append(arpeggio(Square, 0.08, 0.05, 0.4, nC5, nE5, nG5, nC6, nE6),
		beep(nG6, 0.42, 0.2, 0.4)),
	Potion: {
		{Wave: Sine, Freq: 300, Slide: 1.5, Vibrato: 3, VibratoHz: 14, Attack: 0.01, Hold: 0.25, Release: 0.1, Volume: 0.4},
		beep(nC6, 0.3, 0.04, 0.25), beep(nE6, 0.36, 0.06, 0.25),
	},
	Flee: {
		{Wave: Noise, Freq: 1200, Slide: 1, Attack: 0.01, Hold: 0.02, Release: 0.05, Volume: 0.35},
		{Wave: Noise, Freq: 1200, Slide: 1, Delay: 0.1, Attack: 0.01, Hold: 0.02, Release: 0.05, Volume: 0.35},
		{Wave: Noise, Freq: 1200, Slide: 1, Delay: 0.2, Attack: 0.01, Hold: 0.02, Release: 0.05, Volume: 0.35},
	},
	Fall: arpeggio(Triangle, 0.22, 0.15, 0.5, nE5, nC5, nA4, nE4),

	Step: {{Wave: Noise, Freq: 500, Slide: -1, Attack: 0.004, Hold: 0.01, Release: 0.05, Volume: 0.2}},
	Bump: {
		{Wave: Sine, Freq: 110, Slide: -1.5, Attack: 0.002, Hold: 0.03, Release: 0.08, Volume: 0.4},
		{Wave: Noise, Freq: 400, Attack: 0.002, Hold: 0.01, Release: 0.04, Volume: 0.3},
	},
	Door: {
		{Wave: Saw, Freq: 180, Slide: 0.8, Vibrato: 0.8, VibratoHz: 11, Attack: 0.03, Hold: 0.3, Release: 0.12, Volume: 0.3},
		{Wave: Noise, Freq: 300, Delay: 0.4, Attack: 0.002, Hold: 0.02, Release: 0.1, Volume: 0.45},
	},
	Unseal: append(arpeggio(Triangle, 0.05, 0.03, 0.35, nC6, nE6, nG6, nC7, nG6, nC7),
		Tone{Wave: Noise, Freq: 7000, Slide: -1, Attack: 0.05, Hold: 0.1, Release: 0.3, Volume: 0.25}),
	Chest: {
		{Wave: Saw, Freq: 140, Slide: 1, Attack: 0.01, Hold: 0.08, Release: 0.05, Volume: 0.3},
		beep(nE6, 0.14, 0.03, 0.5), beep(nC7, 0.2, 0.03, 0.5), beep(nE6, 0.28, 0.03, 0.5), beep(nC7, 0.34, 0.1, 0.5),
	},
	Zap: {
		{Wave: Square, Freq: 1600, Slide: -5, Duty: 0.2, Vibrato: 4, VibratoHz: 40, Attack: 0.002, Hold: 0.12, Release: 0.08, Volume: 0.8},
	},
	Stairs: arpeggio(Triangle, 0.1, 0.06, 0.4, nG5, nE5, nC5, nG4, nE4, nC4),
	Chomp: {
		// Two bites: a wooden snap and a low growl under them.
		{Wave: Noise, Freq: 1800, Slide: -2, Attack: 0.002, Hold: 0.03, Release: 0.06, Volume: 0.6},
		{Wave: Square, Freq: 160, Slide: -1.5, Duty: 0.5, Attack: 0.002, Hold: 0.05, Release: 0.06, Volume: 0.45},
		{Wave: Noise, Freq: 1800, Slide: -2, Delay: 0.16, Attack: 0.002, Hold: 0.03, Release: 0.06, Volume: 0.6},
		{Wave: Square, Freq: 140, Slide: -1.5, Duty: 0.5, Delay: 0.16, Attack: 0.002, Hold: 0.05, Release: 0.06, Volume: 0.45},
		{Wave: Saw, Freq: 70, Vibrato: 2, VibratoHz: 9, Delay: 0.05, Attack: 0.05, Hold: 0.3, Release: 0.2, Volume: 0.35},
	},
}
