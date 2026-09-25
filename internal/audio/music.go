package audio

import (
	"math"
	"math/rand/v2"
)

// Mood is the feel of a piece of music.
type Mood uint8

const (
	Quiet  Mood = iota // no music
	Title              // the title screen and menus: bright and heroic
	Delve              // exploring a floor: slow and dark, to think over
	Fight              // a battle: quick, with drums
	Boss               // a boss battle: quicker and heavier
	Camp               // a campfire, shrine or shop: calm
	Lament             // the hero has fallen
	numMoods
)

var moodNames = [numMoods]string{"quiet", "title", "delve", "fight", "boss", "camp", "lament"}

func (m Mood) String() string {
	if m >= numMoods {
		return "unknown"
	}
	return moodNames[m]
}

// Moods lists every mood that has music, for tests and tools.
var Moods = []Mood{Title, Delve, Fight, Boss, Camp, Lament}

// Track names a piece of music. The same track is always the same tune:
// the seed picks the key, the tempo, the chords and the melody.
type Track struct {
	Mood Mood
	Seed uint64
}

// Song is a composed track: one loop of notes, ready to render.
type Song struct {
	BPM   float64
	Bars  int
	Notes Sound // every note, placed in the loop by its Delay
	Root  int   // the key, in semitones above A4
	Scale []int // the scale, in semitones above the root
}

// Loop is the length of one loop in seconds.
func (s Song) Loop() float64 { return float64(s.Bars) * 4 * 60 / s.BPM }

// Scales, in semitones above the root.
var (
	major      = []int{0, 2, 4, 5, 7, 9, 11}
	minor      = []int{0, 2, 3, 5, 7, 8, 10}
	dorian     = []int{0, 2, 3, 5, 7, 9, 10}
	harmonic   = []int{0, 2, 3, 5, 7, 8, 11}
	phrygian   = []int{0, 1, 3, 5, 7, 8, 10}
	mixolydian = []int{0, 2, 4, 5, 7, 9, 10}
)

// style is how a mood is arranged.
type style struct {
	bpm    [2]float64 // the tempo range
	scales [][]int
	progs  [][]int // chord progressions, as scale degrees from 0, one chord a bar
	bars   int
	lead   Wave
	leadV  float64 // lead volume
	busy   float64 // how many sixteenths start a lead note, about
	rest   bool    // the lead sits out every other phrase
	arp    Wave    // the wave of the arpeggio, or Noise for none
	arpV   float64
	pad    bool // hold each chord as a soft pad instead
	bass   int  // 0 long notes, 1 eighths, 2 pumping octaves
	bassV  float64
	drums  int // 0 none, 1 soft hats, 2 a full kit
	lower  int // semitones the whole song is lowered by
}

var styles = [numMoods]style{
	Title: {
		bpm: [2]float64{108, 118}, scales: [][]int{major, mixolydian},
		progs: [][]int{{0, 4, 5, 3}, {0, 3, 4, 4}, {0, 5, 3, 4}, {0, 6, 3, 0}},
		bars:  8, lead: Square, leadV: 0.34, busy: 0.45,
		arp: Square, arpV: 0.1, bass: 1, bassV: 0.5, drums: 2,
	},
	Delve: {
		bpm: [2]float64{76, 88}, scales: [][]int{minor, dorian, harmonic},
		progs: [][]int{{0, 5, 2, 6}, {0, 3, 0, 4}, {0, 6, 5, 6}, {0, 3, 5, 4}},
		bars:  8, lead: Triangle, leadV: 0.36, busy: 0.22, rest: true,
		arp: Triangle, arpV: 0.16, bass: 0, bassV: 0.5, drums: 1, lower: 3,
	},
	Fight: {
		bpm: [2]float64{132, 142}, scales: [][]int{minor, harmonic, dorian},
		progs: [][]int{{0, 5, 6, 0}, {0, 3, 4, 0}, {0, 6, 5, 4}, {0, 5, 3, 4}},
		bars:  8, lead: Square, leadV: 0.28, busy: 0.5,
		arp: Square, arpV: 0.09, bass: 2, bassV: 0.5, drums: 2,
	},
	Boss: {
		bpm: [2]float64{146, 156}, scales: [][]int{phrygian, harmonic},
		progs: [][]int{{0, 1, 0, 6}, {0, 5, 1, 4}, {0, 1, 5, 4}},
		bars:  8, lead: Saw, leadV: 0.2, busy: 0.55,
		arp: Square, arpV: 0.09, bass: 2, bassV: 0.55, drums: 2, lower: 5,
	},
	Camp: {
		bpm: [2]float64{68, 76}, scales: [][]int{major},
		progs: [][]int{{0, 3, 0, 4}, {0, 5, 3, 4}, {0, 3, 5, 4}},
		bars:  8, lead: Sine, leadV: 0.34, busy: 0.25, rest: true,
		arp: Triangle, arpV: 0.2, bass: 0, bassV: 0.45,
	},
	Lament: {
		bpm: [2]float64{62, 68}, scales: [][]int{minor, harmonic},
		progs: [][]int{{0, 5, 3, 4}, {0, 3, 5, 4}},
		bars:  4, lead: Triangle, leadV: 0.4, busy: 0.2,
		arp: Noise, pad: true, arpV: 0.12, bass: 0, bassV: 0.45, lower: 3,
	},
}

// stepsPerBar is how many sixteenth notes are in a 4/4 bar.
const stepsPerBar = 16

// Compose writes the tune for a track. It is quick, and the same track
// always gives the same notes.
func Compose(t Track) Song {
	if t.Mood == Quiet || t.Mood >= numMoods {
		return Song{BPM: 120, Bars: 1}
	}
	st := &styles[t.Mood]
	rng := rand.New(rand.NewPCG(t.Seed, uint64(t.Mood)*0x9e3779b97f4a7c15+1))
	s := Song{
		BPM:   st.bpm[0] + rng.Float64()*(st.bpm[1]-st.bpm[0]),
		Bars:  st.bars,
		Root:  rng.IntN(7) - 3 - st.lower, // around A4
		Scale: st.scales[rng.IntN(len(st.scales))],
	}
	prog := st.progs[rng.IntN(len(st.progs))]
	c := composer{Song: &s, st: st, rng: rng, step: 60 / s.BPM / 4}
	for bar := 0; bar < s.Bars; bar++ {
		chord := prog[bar%len(prog)]
		c.bass(bar, chord)
		c.harmony(bar, chord)
		c.drums(bar)
	}
	c.melody(prog)
	return s
}

// composer holds what Compose needs while it writes a song.
type composer struct {
	*Song
	st   *style
	rng  *rand.Rand
	step float64 // seconds per sixteenth
}

// pitch returns the semitones above A4 of scale degree d, counted from the
// root, going up through the octaves.
func (c *composer) pitch(d int) int {
	n := len(c.Scale)
	oct := int(math.Floor(float64(d) / float64(n)))
	return c.Root + 12*oct + c.Scale[d-oct*n]
}

// chordTones returns the degrees of the triad on degree d.
func chordTones(d int) [3]int { return [3]int{d, d + 2, d + 4} }

// add places a note of semitone k at sixteenth step, lasting steps.
func (c *composer) add(w Wave, k int, step, steps float64, vol, duty float64) {
	hold := max(0.01, steps*c.step*0.75-0.01)
	c.Notes = append(c.Notes, Tone{
		Wave: w, Freq: note(float64(k)), Duty: duty,
		Delay: step * c.step, Attack: 0.006, Hold: hold, Release: min(0.12, steps*c.step*0.5),
		Volume: vol,
	})
}

func (c *composer) bass(bar, chord int) {
	st := c.st
	at := float64(bar * stepsPerBar)
	root := c.pitch(chord) - 24
	fifth := c.pitch(chord+4) - 24
	switch st.bass {
	case 0: // long notes on beats one and three
		c.add(Triangle, root, at, 8, st.bassV, 0)
		c.add(Triangle, fifth, at+8, 8, st.bassV*0.8, 0)
	case 1: // eighths, root and fifth
		for i := 0; i < 8; i++ {
			k := root
			if i%4 == 3 {
				k = fifth
			}
			c.add(Triangle, k, at+float64(2*i), 2, st.bassV, 0)
		}
	case 2: // pumping octaves
		for i := 0; i < 8; i++ {
			k := root
			if i%2 == 1 {
				k += 12
			}
			c.add(Triangle, k, at+float64(2*i), 2, st.bassV, 0)
		}
	}
}

func (c *composer) harmony(bar, chord int) {
	st := c.st
	at := float64(bar * stepsPerBar)
	tones := chordTones(chord)
	if st.pad {
		for _, d := range tones {
			c.add(Triangle, c.pitch(d)-12, at, stepsPerBar, st.arpV, 0)
		}
		return
	}
	if st.arp == Noise {
		return
	}
	// An arpeggio up and down the chord: sixteenths in quick songs,
	// eighths in slow ones.
	every := 1
	if c.BPM < 100 {
		every = 2
	}
	order := []int{0, 1, 2, 3, 2, 1}
	for i, s := 0, 0; s < stepsPerBar; i, s = i+1, s+every {
		o := order[i%len(order)]
		k := c.pitch(tones[o%3]) + 12*(o/3) - 12
		c.add(st.arp, k, at+float64(s), float64(every), st.arpV, 0.125)
	}
}

func (c *composer) drums(bar int) {
	at := float64(bar * stepsPerBar)
	t := func(step float64) float64 { return (at + step) * c.step }
	hat := func(step, vol float64) {
		c.Notes = append(c.Notes, Tone{Wave: Noise, Freq: 9000, Delay: t(step), Attack: 0.001, Hold: 0.005, Release: 0.035, Volume: vol})
	}
	switch c.st.drums {
	case 1:
		// A soft hat on the off beats, like dripping water.
		for s := 2.0; s < stepsPerBar; s += 4 {
			hat(s, 0.08)
		}
	case 2:
		for s := 0.0; s < stepsPerBar; s += 2 {
			hat(s, 0.1)
		}
		for _, s := range []float64{0, 8} {
			c.Notes = append(c.Notes, Tone{Wave: Sine, Freq: 150, Slide: -4, Delay: t(s), Attack: 0.001, Hold: 0.03, Release: 0.12, Volume: 0.7})
		}
		if bar%2 == 1 {
			c.Notes = append(c.Notes, Tone{Wave: Sine, Freq: 150, Slide: -4, Delay: t(10), Attack: 0.001, Hold: 0.03, Release: 0.12, Volume: 0.55})
		}
		for _, s := range []float64{4, 12} {
			c.Notes = append(c.Notes,
				Tone{Wave: Noise, Freq: 3500, Slide: -1, Delay: t(s), Attack: 0.001, Hold: 0.01, Release: 0.1, Volume: 0.3},
				Tone{Wave: Triangle, Freq: 190, Slide: -2, Delay: t(s), Attack: 0.001, Hold: 0.01, Release: 0.06, Volume: 0.3})
		}
	}
}

// Rhythms are where lead notes start in two bars, as sixteenth steps.
var rhythms = [][]int{
	{0, 4, 6, 8, 12, 16, 20, 24},
	{0, 3, 6, 8, 12, 14, 16, 22, 24},
	{0, 2, 4, 8, 10, 12, 16, 24, 28},
	{0, 6, 8, 12, 16, 18, 20, 24},
	{0, 4, 8, 10, 12, 14, 16, 20, 24, 28},
	{0, 2, 3, 4, 8, 12, 16, 18, 19, 20, 24},
}

// melody writes the lead over the chords: a two-bar phrase, played again
// with a new ending, then a contrasting phrase, then the first again,
// resolving home.
func (c *composer) melody(prog []int) {
	st := c.st
	phrases := c.Bars / 2
	var first []leadNote
	for p := 0; p < phrases; p++ {
		if st.rest && p%2 == 1 && p != phrases-1 {
			continue // the lead rests and lets the dungeon breathe
		}
		chords := [2]int{prog[(2*p)%len(prog)], prog[(2*p+1)%len(prog)]}
		var ph []leadNote
		switch {
		case p == 0 || first == nil:
			ph = c.phrase(chords, c.rhythm(), 4)
			first = ph
		case p == 2 && phrases > 3:
			ph = c.phrase(chords, c.rhythm(), 6) // higher, for contrast
		default:
			ph = c.vary(first, chords)
		}
		if p == phrases-1 {
			ph = c.resolve(ph)
		}
		for i, n := range ph {
			end := 32
			if i+1 < len(ph) {
				end = ph[i+1].at
			}
			if n.deg == restDeg {
				continue
			}
			c.addLead(c.pitch(n.deg), float64(p*32+n.at), float64(end-n.at))
		}
	}
}

func (c *composer) addLead(k int, step, steps float64) {
	st := c.st
	hold := max(0.02, steps*c.step*0.8-0.02)
	t := Tone{
		Wave: st.lead, Freq: note(float64(k)), Duty: 0.25,
		Delay: step * c.step, Attack: 0.01, Hold: hold, Release: min(0.15, steps*c.step*0.6),
		Volume: st.leadV,
	}
	if steps*c.step > 0.3 {
		t.Vibrato, t.VibratoHz = 0.25, 5.5 // long notes sing
	}
	c.Notes = append(c.Notes, t)
}

// leadNote is a lead note in a phrase: its step within the two bars and
// its scale degree.
type leadNote struct{ at, deg int }

// restDeg marks a silent step in a phrase.
const restDeg = math.MinInt32

// rhythm picks note starts for a phrase, thinned for calm moods.
func (c *composer) rhythm() []int {
	r := rhythms[c.rng.IntN(len(rhythms))]
	out := []int{r[0]}
	for _, s := range r[1:] {
		if s%8 == 0 || c.rng.Float64() < c.st.busy*2 {
			out = append(out, s)
		}
	}
	return out
}

// phrase walks the scale over two chords, landing on a chord tone on the
// strong beats. centre is the degree it stays near.
func (c *composer) phrase(chords [2]int, rhythm []int, centre int) []leadNote {
	deg := centre + c.rng.IntN(3) - 1
	var out []leadNote
	for _, at := range rhythm {
		chord := chords[at/16]
		switch r := c.rng.Float64(); {
		case r < 0.55:
			deg += 1 - 2*c.rng.IntN(2) // a step
		case r < 0.8:
			deg += (2 + c.rng.IntN(2)) * (1 - 2*c.rng.IntN(2)) // a leap
		}
		if at%8 == 0 {
			deg = nearestChordTone(deg, chord, len(c.Scale))
		}
		deg = max(centre-4, min(centre+6, deg))
		out = append(out, leadNote{at, deg})
	}
	return out
}

// vary repeats a phrase, moved onto new chords, with the last notes new.
func (c *composer) vary(ph []leadNote, chords [2]int) []leadNote {
	out := make([]leadNote, len(ph))
	copy(out, ph)
	for i := range out {
		n := &out[i]
		if n.at >= 16 && c.rng.IntN(2) == 0 {
			n.deg += 1 - 2*c.rng.IntN(2)
		}
		if n.at%8 == 0 {
			n.deg = nearestChordTone(n.deg, chords[n.at/16], len(c.Scale))
		}
	}
	return out
}

// resolve ends a phrase on the home note, held.
func (c *composer) resolve(ph []leadNote) []leadNote {
	var out []leadNote
	for _, n := range ph {
		if n.at < 24 {
			out = append(out, n)
		}
	}
	last := 4
	if len(out) > 0 {
		last = out[len(out)-1].deg
	}
	n := len(c.Scale)
	home := int(math.Round(float64(last)/float64(n))) * n
	return append(out, leadNote{24, home})
}

// nearestChordTone moves degree d to the closest note of the triad on chord.
func nearestChordTone(d, chord, n int) int {
	best, dist := d, math.MaxInt
	for oct := -2; oct <= 2; oct++ {
		for _, t := range chordTones(chord) {
			c := t + oct*n
			if a := abs(c - d); a < dist {
				best, dist = c, a
			}
		}
	}
	return best
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// RenderLoop mixes one loop of the song into mono samples between -1 and
// 1. Notes that ring past the end wrap round to the start, so the loop
// plays on without a seam. yield, if not nil, is called now and then so a
// long render can give other work a turn.
func RenderLoop(s Song, rate int, yield func()) []float32 {
	n := int(math.Round(s.Loop() * float64(rate)))
	mix := make([]float64, n+rate) // a second for tails
	for i, t := range s.Notes {
		t.render(mix, rate, uint32(i)*0x9e3779b9+0x6d2b79f5)
		if yield != nil && i%16 == 15 {
			yield()
		}
	}
	for i := n; i < len(mix); i++ {
		mix[i-n] += mix[i]
	}
	peak := 0.0
	for _, v := range mix[:n] {
		peak = max(peak, math.Abs(v))
	}
	gain := 1.0
	if peak > 0.9 {
		gain = 0.9 / peak
	}
	out := make([]float32, n)
	for i, v := range mix[:n] {
		out[i] = float32(v * gain)
	}
	return out
}
