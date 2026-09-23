// Package audio makes the game's sound effects in code, sfxr style: each
// sound is a few tones of simple waves with a volume envelope and a pitch
// slide. It only makes samples; the game plays them.
package audio

import (
	"encoding/binary"
	"math"
)

// SampleRate is the rate sounds are rendered and played at.
const SampleRate = 44100

// Wave is the shape of a tone's waveform.
type Wave uint8

const (
	Square Wave = iota
	Triangle
	Saw
	Sine
	Noise
)

// loudness evens out how loud each wave sounds at the same volume.
var loudness = [...]float64{Square: 0.45, Triangle: 1, Saw: 0.55, Sine: 1, Noise: 0.6}

// Tone is one note of a sound effect.
type Tone struct {
	Wave  Wave
	Freq  float64 // starting pitch in Hz; for noise it sets how rough it sounds
	Slide float64 // pitch change in octaves per second; negative falls
	Duty  float64 // square wave pulse width, 0 to 1; 0 means 0.5

	Vibrato   float64 // pitch wobble depth in semitones
	VibratoHz float64 // pitch wobble speed

	Delay   float64 // when the tone starts within the sound, in seconds
	Attack  float64 // seconds to fade in
	Hold    float64 // seconds at full volume
	Release float64 // seconds to fade out
	Volume  float64 // 0 to 1
}

// Len returns when the tone ends, in seconds from the start of the sound.
func (t Tone) Len() float64 { return t.Delay + t.Attack + t.Hold + t.Release }

// Sound is a sound effect: tones that play in sequence or together,
// depending on their delays.
type Sound []Tone

// Len returns the sound's length in seconds.
func (s Sound) Len() float64 {
	n := 0.0
	for _, t := range s {
		n = max(n, t.Len())
	}
	return n
}

// Render mixes the sound into mono samples between -1 and 1. The same sound
// always gives the same samples.
func Render(s Sound, rate int) []float32 {
	out := make([]float32, int(math.Ceil(s.Len()*float64(rate))))
	mix := make([]float64, len(out))
	for i, t := range s {
		t.render(mix, rate, uint32(i)*0x9e3779b9+0x6d2b79f5)
	}
	for i, v := range mix {
		out[i] = float32(max(-1, min(1, v)))
	}
	return out
}

// render adds the tone into mix. seed drives the noise wave.
func (t Tone) render(mix []float64, rate int, seed uint32) {
	start := int(t.Delay * float64(rate))
	n := int((t.Attack + t.Hold + t.Release) * float64(rate))
	duty := t.Duty
	if duty <= 0 || duty >= 1 {
		duty = 0.5
	}
	vol := t.Volume * loudness[t.Wave]
	nyquist := float64(rate) / 2
	phase := 0.0
	noise := 0.0
	rng := seed | 1
	for i := 0; i < n && start+i < len(mix); i++ {
		secs := float64(i) / float64(rate)
		f := t.Freq * math.Exp2(t.Slide*secs)
		if t.Vibrato != 0 {
			f *= math.Exp2(t.Vibrato / 12 * math.Sin(2*math.Pi*t.VibratoHz*secs))
		}
		f = max(20, min(nyquist, f))

		var v float64
		switch t.Wave {
		case Square:
			v = 1
			if phase >= duty {
				v = -1
			}
		case Triangle:
			v = 4*math.Abs(phase-0.5) - 1
		case Saw:
			v = 2*phase - 1
		case Sine:
			v = math.Sin(2 * math.Pi * phase)
		case Noise:
			v = noise
		}
		mix[start+i] += v * vol * t.envelope(secs)

		// Noise picks a new random level twice per cycle, so a higher
		// pitch sounds hissier.
		phase += f / float64(rate)
		for phase >= 0.5 && t.Wave == Noise {
			rng ^= rng << 13
			rng ^= rng >> 17
			rng ^= rng << 5
			noise = float64(rng)/math.MaxUint32*2 - 1
			phase -= 0.5
		}
		phase -= math.Floor(phase)
	}
}

// envelope is the tone's volume at secs from its start: a straight fade in,
// a hold, and a curved fade out that ends at zero, so there is no click.
func (t Tone) envelope(secs float64) float64 {
	switch {
	case secs < t.Attack:
		return secs / t.Attack
	case secs < t.Attack+t.Hold:
		return 1
	case t.Release <= 0:
		return 0
	}
	r := 1 - (secs-t.Attack-t.Hold)/t.Release
	if r <= 0 {
		return 0
	}
	return r * r
}

// Encode turns samples into the little-endian 32-bit float bytes the audio
// device plays.
func Encode(samples []float32) []byte {
	b := make([]byte, 4*len(samples))
	for i, s := range samples {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(s))
	}
	return b
}
