package audio

// World is which world a piece of music is for. Each world has its own
// key, scale, tempo range and instruments, so the music says where you
// are; the track's seed still writes a new tune for every floor. NoWorld
// is the music as it was before worlds had their own.
//
// The world shapes the Delve music fully and gives the Fight and Boss
// music its key, scale and lead. The other moods are the same everywhere.
type World uint8

const (
	NoWorld World = iota
	Crypt
	Cellars
	Caves
	Forge
	Ice
	Library
	Garden
	Workshop
	Vaults
	Tomb
	Temple
	Candy
	Mushroom
	Observatory
	numWorlds
)

// Worlds lists every world with its own music, for tests and tools.
var Worlds = []World{Crypt, Cellars, Caves, Forge, Ice, Library, Garden, Workshop, Vaults, Tomb, Temple, Candy, Mushroom, Observatory}

// worldNames are the names of the worlds in pkg/proc, so the game can pick
// a world's music by its name. A test checks they match proc.Themes.
var worldNames = [numWorlds]string{
	NoWorld:     "",
	Crypt:       "The Crypt",
	Cellars:     "Mossy Cellars",
	Caves:       "Flooded Caves",
	Forge:       "Lava Forge",
	Ice:         "Ice Halls",
	Library:     "Whispering Library",
	Garden:      "Sky Garden",
	Workshop:    "Clockwork Workshop",
	Vaults:      "Amethyst Vaults",
	Tomb:        "Sandstone Tomb",
	Temple:      "Sunken Temple",
	Candy:       "Candy Caverns",
	Mushroom:    "Mushroom Forest",
	Observatory: "Star Observatory",
}

var worldIDs = [numWorlds]string{"none", "crypt", "cellars", "caves", "forge", "ice", "library", "garden", "workshop", "vaults", "tomb", "temple", "candy", "mushroom", "observatory"}

func (w World) String() string {
	if w >= numWorlds {
		return "unknown"
	}
	return worldIDs[w]
}

// WorldNamed returns the world with the given name, as pkg/proc names it,
// or NoWorld for a name it doesn't know.
func WorldNamed(name string) World {
	if name == "" {
		return NoWorld
	}
	for w, n := range worldNames {
		if n == name {
			return World(w)
		}
	}
	return NoWorld
}

// More scales, in semitones above the root.
var (
	lydian     = []int{0, 2, 4, 6, 7, 9, 11}
	pentMinor  = []int{0, 3, 5, 7, 10}
	pentMajor  = []int{0, 2, 4, 7, 9}
	hirajoshi  = []int{0, 2, 3, 7, 8}
	phrygDom   = []int{0, 1, 4, 5, 7, 8, 10}
	pentChords = [][]int{{0, 3, 1, 2}, {0, 2, 3, 1}, {0, 1, 3, 2}}
)

// palette is a world's sound: what it changes in a mood's style.
type palette struct {
	root  int   // the Delve key, in semitones above A4
	scale []int // the scale for every mood the world shapes
	bpm   [2]float64
	progs [][]int // Delve chord progressions; nil keeps the mood's

	lead     Wave
	leadV    float64
	leadDuty float64 // for a square lead; 0 means 0.25
	busy     float64
	vibrato  float64 // semitones on long lead notes; 0 means 0.25

	arp      Wave // or Noise for none
	arpV     float64
	arpEvery int  // sixteenths per arpeggio note; 0 picks by tempo
	pad      bool // hold the chords instead of an arpeggio
	padW     Wave
	bass     int
	drums    int
}

// palettes are the worlds' sounds. Delve roots sit between C and C an
// octave up around A4, like the mood's own range, and tempos stay at 72
// or more so a loop is never longer than the slowest Delve loop was.
var palettes = [numWorlds]palette{
	// Solemn: an organ holding harmonic minor chords, a slow singing lead.
	Crypt: {
		root: -5, scale: harmonic, bpm: [2]float64{72, 78},
		progs: [][]int{{0, 5, 3, 4}, {0, 3, 4, 0}, {0, 5, 6, 4}},
		lead:  Triangle, leadV: 0.36, busy: 0.2,
		pad: true, padW: Square, arpV: 0.07, bass: 0, drums: 1,
	},
	// Earthy folk: a reedy square lead over a walking bass, in Dorian.
	Cellars: {
		root: -2, scale: dorian, bpm: [2]float64{84, 92},
		progs: [][]int{{0, 3, 0, 4}, {0, 6, 3, 0}, {0, 3, 6, 4}},
		lead:  Square, leadV: 0.26, leadDuty: 0.125, busy: 0.3,
		arp: Triangle, arpV: 0.14, bass: 1, drums: 1,
	},
	// Water: soft sines rippling in eighths on a minor pentatonic, no
	// drums under the drips.
	Caves: {
		root: -7, scale: pentMinor, bpm: [2]float64{74, 80}, progs: pentChords,
		lead: Sine, leadV: 0.36, busy: 0.22, vibrato: 0.35,
		arp: Sine, arpV: 0.18, arpEvery: 2, bass: 0, drums: 0,
	},
	// Heavy: Phrygian, a buzzing saw lead, eighth-note bass and hammer
	// blows on the beat.
	Forge: {
		root: -9, scale: phrygian, bpm: [2]float64{86, 94},
		progs: [][]int{{0, 1, 0, 6}, {0, 6, 5, 1}, {0, 1, 5, 0}},
		lead:  Saw, leadV: 0.2, busy: 0.25,
		arp: Square, arpV: 0.08, bass: 1, drums: 3,
	},
	// Cold and glassy: Lydian, high and bright, a quick sparkling
	// arpeggio over long bass notes, no drums.
	Ice: {
		root: 2, scale: lydian, bpm: [2]float64{72, 78},
		progs: [][]int{{0, 1, 0, 4}, {0, 1, 5, 4}, {0, 4, 1, 0}},
		lead:  Sine, leadV: 0.32, busy: 0.2,
		arp: Triangle, arpV: 0.12, arpEvery: 1, bass: 0, drums: 0,
	},
	// Hushed and warm: a soft sine pad in major, a sparse lead, nothing
	// to disturb the readers.
	Library: {
		root: -4, scale: major, bpm: [2]float64{72, 76},
		progs: [][]int{{0, 3, 5, 4}, {0, 5, 3, 4}, {0, 2, 3, 4}},
		lead:  Triangle, leadV: 0.34, busy: 0.15,
		pad: true, padW: Sine, arpV: 0.13, bass: 0, drums: 0,
	},
	// Light and airy: a major pentatonic, a bright thin square lead and
	// gentle hats.
	Garden: {
		root: 3, scale: pentMajor, bpm: [2]float64{92, 98}, progs: pentChords,
		lead: Square, leadV: 0.24, leadDuty: 0.125, busy: 0.35,
		arp: Triangle, arpV: 0.13, bass: 1, drums: 1,
	},
	// Mechanical: Mixolydian, a square arpeggio in steady sixteenths and
	// a bass in eighths, like gears turning.
	Workshop: {
		root: -1, scale: mixolydian, bpm: [2]float64{100, 106},
		progs: [][]int{{0, 6, 3, 0}, {0, 3, 6, 4}, {0, 4, 6, 3}},
		lead:  Square, leadV: 0.24, leadDuty: 0.5, busy: 0.3,
		arp: Square, arpV: 0.08, arpEvery: 1, bass: 1, drums: 1,
	},
	// Crystal: a Hirajoshi scale, a sine lead with a wide shimmer, a
	// triangle arpeggio. The ambience's chimes ring in the same scale.
	Vaults: {
		root: 1, scale: hirajoshi, bpm: [2]float64{74, 80}, progs: pentChords,
		lead: Sine, leadV: 0.34, busy: 0.22, vibrato: 0.4,
		arp: Triangle, arpV: 0.15, arpEvery: 2, bass: 0, drums: 0,
	},
	// Desert: Phrygian dominant, a nasal square lead and a frame drum
	// (low doum, light tek).
	Tomb: {
		root: -6, scale: phrygDom, bpm: [2]float64{80, 86},
		progs: [][]int{{0, 1, 0, 6}, {0, 3, 1, 0}, {0, 6, 1, 0}},
		lead:  Square, leadV: 0.24, leadDuty: 0.25, busy: 0.28,
		arp: Triangle, arpV: 0.12, bass: 0, drums: 4,
	},
	// Underwater: slow Dorian under a soft triangle pad, a sine lead that
	// wavers like light through water, no drums.
	Temple: {
		root: -9, scale: dorian, bpm: [2]float64{74, 80},
		progs: [][]int{{0, 3, 4, 0}, {0, 6, 3, 4}, {0, 4, 3, 0}},
		lead:  Sine, leadV: 0.34, busy: 0.2, vibrato: 0.35,
		pad: true, padW: Triangle, arpV: 0.12, bass: 0, drums: 0,
	},
	// Sweet and bouncy: a quick major tune, a square lead, a sparkling
	// triangle arpeggio in sixteenths and a light beat.
	Candy: {
		root: 0, scale: major, bpm: [2]float64{104, 110},
		progs: [][]int{{0, 4, 5, 3}, {0, 3, 4, 0}, {0, 5, 3, 4}},
		lead:  Square, leadV: 0.24, leadDuty: 0.25, busy: 0.38,
		arp: Triangle, arpV: 0.12, arpEvery: 1, bass: 1, drums: 1,
	},
	// Whimsical and a little mysterious: a minor pentatonic, a hopping
	// triangle lead, a soft sine arpeggio in eighths over a walking bass
	// and a light beat.
	Mushroom: {
		root: -3, scale: pentMinor, bpm: [2]float64{86, 92}, progs: pentChords,
		lead: Triangle, leadV: 0.34, busy: 0.3,
		arp: Sine, arpV: 0.15, arpEvery: 2, bass: 1, drums: 1,
	},
	// Wonder: slow and spacious Lydian under a held sine pad, a triangle
	// lead with a gentle shimmer, no drums.
	Observatory: {
		root: -3, scale: lydian, bpm: [2]float64{72, 76},
		progs: [][]int{{0, 1, 4, 0}, {0, 4, 1, 5}, {0, 5, 1, 0}},
		lead:  Triangle, leadV: 0.34, busy: 0.18, vibrato: 0.3,
		pad: true, padW: Sine, arpV: 0.12, bass: 0, drums: 0,
	},
}

// worldStyle returns the mood's style as world w plays it, and the key to
// use instead of the drawn one (ok false keeps the drawn key).
func worldStyle(st style, m Mood, w World) (style, int, bool) {
	if w == NoWorld || w >= numWorlds {
		return st, 0, false
	}
	p := &palettes[w]
	switch m {
	case Delve:
		st.bpm = p.bpm
		st.scales = [][]int{p.scale}
		if p.progs != nil {
			st.progs = p.progs
		}
		st.lead, st.leadV, st.leadDuty, st.busy, st.vibrato = p.lead, p.leadV, p.leadDuty, p.busy, p.vibrato
		st.arp, st.arpV, st.arpEvery = p.arp, p.arpV, p.arpEvery
		st.pad, st.padW = p.pad, p.padW
		st.bass, st.drums = p.bass, p.drums
		if p.pad {
			st.arp = Noise
		}
		return st, p.root, true
	case Fight, Boss:
		// The battle keeps its tempo, bass and drums, and plays in the
		// world's key and scale with the world's lead. Pentatonic worlds
		// keep chords that fit a five-note scale.
		st.scales = [][]int{p.scale}
		if len(p.scale) < 7 {
			st.progs = pentChords
		}
		st.lead, st.leadDuty, st.vibrato = p.lead, p.leadDuty, p.vibrato
		// A soft lead (sine or triangle) needs more volume than a buzzy
		// one to sit over the drums.
		switch p.lead {
		case Square:
			st.leadV = 0.28
		case Saw:
			st.leadV = 0.2
		default:
			st.leadV = 0.34
		}
		root := p.root + 3 - st.lower // as high above the Delve key as the mood is
		return st, root, true
	}
	return st, 0, false
}
