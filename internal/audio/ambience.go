package audio

import (
	"math"
	"math/rand/v2"
)

// Ambience is a quiet layer of sound mixed under the Delve music, so each
// world sounds like somewhere. It is made of sparse, gentle events and
// soft noise beds, and it is never loud, sudden or bright.
type Ambience uint8

const (
	None     Ambience = iota // no ambience: today's music, unchanged
	Dust                     // low wind in dusty halls
	Crickets                 // chirps in the dark
	Drips                    // water dripping
	Rumble                   // a low rumble and the crackle of fire
	Wind                     // icy wind
	Pages                    // rustling pages and a soft candle
	Birds                    // distant birds
	Ticking                  // a clock ticking, calmly
	Chimes                   // soft crystal chimes
	Sand                     // soft wind over sand
	numAmbiences
)

var ambienceNames = [numAmbiences]string{"none", "dust", "crickets", "drips", "rumble", "wind", "pages", "birds", "ticking", "chimes", "sand"}

func (a Ambience) String() string {
	if a >= numAmbiences {
		return "unknown"
	}
	return ambienceNames[a]
}

// Ambiences lists every ambience that makes a sound, for tests and tools.
var Ambiences = []Ambience{Dust, Crickets, Drips, Rumble, Wind, Pages, Birds, Ticking, Chimes, Sand}

// ambienceCap is the most the ambience layer may reach, against music that
// peaks near 0.9: it sits well beneath it.
const ambienceCap = 0.1

// RenderAmbience renders just the ambience layer of a song, one loop long
// and seamless when repeated. It is empty for a song without one.
func RenderAmbience(s Song, rate int, yield func()) []float32 {
	n := int(math.Round(s.Loop() * float64(rate)))
	if s.Ambience == None || s.Ambience >= numAmbiences || n <= 0 {
		return nil
	}
	l := newLayer(n, rate, s.Seed, s.Ambience, yield)
	switch s.Ambience {
	case Dust:
		l.bed(0.004, 0, 2, 0.6, 0.05)
	case Crickets:
		l.crickets(2.8)
	case Drips:
		l.drips(2.2)
	case Rumble:
		l.bed(0.006, 0, 3, 0.5, 0.07)
		l.crackle(5)
	case Wind:
		l.bed(0.07, 0.008, 3, 0.8, 0.07)
	case Pages:
		l.pages(3.5)
		l.crackle(1.2)
	case Birds:
		l.birds(3.2)
	case Ticking:
		l.ticks()
	case Chimes:
		l.chimes(s, 4)
	case Sand:
		l.bed(0.04, 0.005, 2, 0.7, 0.045)
		l.bed(0.2, 0.05, 5, 0.9, 0.012)
	}
	peak := 0.0
	for _, v := range l.buf {
		peak = max(peak, math.Abs(v))
	}
	g := 1.0
	if peak > ambienceCap {
		g = ambienceCap / peak
	}
	out := make([]float32, n)
	for i, v := range l.buf {
		out[i] = float32(v * g)
	}
	return out
}

// mixAmbience adds the ambience layer to a rendered loop, keeping the
// result inside -1 and 1.
func mixAmbience(out []float32, s Song, rate int, yield func()) {
	amb := RenderAmbience(s, rate, yield)
	peak := float32(0)
	for i, v := range amb {
		out[i] += v
		peak = max(peak, float32(math.Abs(float64(out[i]))))
	}
	if peak > 0.98 {
		g := 0.98 / peak
		for i := range out {
			out[i] *= g
		}
	}
}

// layer is the ambience being built: a loop of samples that events and
// beds wrap round the end of.
type layer struct {
	buf   []float64
	n     int
	rate  float64
	rng   *rand.Rand
	yield func()
}

func newLayer(n, rate int, seed uint64, a Ambience, yield func()) *layer {
	return &layer{
		buf: make([]float64, n), n: n, rate: float64(rate), yield: yield,
		rng: rand.New(rand.NewPCG(seed^0xa5a5a5a5, uint64(a)*0x9e3779b97f4a7c15+7)),
	}
}

func (l *layer) pause() {
	if l.yield != nil {
		l.yield()
	}
}

// add puts v at sample i, wrapping round the loop.
func (l *layer) add(i int, v float64) {
	l.buf[((i%l.n)+l.n)%l.n] += v
}

// times picks about perSec events a second at random moments.
func (l *layer) times(perSec float64) []int {
	count := max(1, int(math.Round(perSec*float64(l.n)/l.rate)))
	out := make([]int, count)
	for i := range out {
		out[i] = l.rng.IntN(l.n)
	}
	return out
}

// bed adds filtered noise that swells and fades slowly. A one-pole low
// pass at coefficient lo, minus another at hi when hi is above zero, gives
// a rumble or a band of wind. The swell goes round whole times (cycles) in
// a loop, so it is seamless; depth is how much it swells. level is the
// bed's peak.
func (l *layer) bed(lo, hi float64, cycles int, depth, level float64) {
	// Beds are soft and dull, so make them at a fraction of the rate and
	// draw straight lines between the samples: much less to compute.
	const slow = 8
	m := (l.n + slow - 1) / slow
	k := float64(l.n) / float64(m) // old samples per slow sample
	lo = 1 - math.Pow(1-lo, k)
	if hi > 0 {
		hi = 1 - math.Pow(1-hi, k)
	}
	white := make([]float64, m)
	for i := range white {
		white[i] = l.rng.Float64()*2 - 1
	}
	// Run the filters round the loop twice and keep the second time, so
	// the end flows into the start.
	var a, b float64
	out := make([]float64, m)
	for pass := 0; pass < 2; pass++ {
		for i, w := range white {
			a += lo * (w - a)
			v := a
			if hi > 0 {
				b += hi * (w - b)
				v -= b
			}
			out[i] = v
		}
	}
	l.pause()
	phase := l.rng.Float64() * 2 * math.Pi
	w := 2 * math.Pi * float64(cycles) / float64(m)
	for i := range out {
		out[i] *= 1 - depth + depth*(0.5+0.5*math.Sin(w*float64(i)+phase))
	}
	peak := 0.0
	tmp := make([]float64, l.n)
	for i := range tmp {
		pos := float64(i) / k
		j := int(pos)
		f := pos - float64(j)
		v := out[j%m]*(1-f) + out[(j+1)%m]*f
		tmp[i] = v
		peak = max(peak, math.Abs(v))
	}
	if peak == 0 {
		return
	}
	for i, v := range tmp {
		l.buf[i] += v / peak * level
	}
}

// ping adds a decaying sine: a drip, a chime, a bird's note. It glides
// from f to f*glide over its life and fades with a time constant of decay
// seconds, over length seconds, easing in over a few milliseconds.
func (l *layer) ping(at int, f, glide, decay, length, amp float64) {
	n := int(min(length, decay*8) * l.rate)
	attack := 0.004 * l.rate
	phase := 0.0
	env := amp
	fall := math.Exp(-1 / (decay * l.rate))
	if glide == 1 {
		// A steady pitch: turn a point round a circle, no sines needed.
		c, s := math.Cos(2*math.Pi*f/l.rate), math.Sin(2*math.Pi*f/l.rate)
		x, y := 0.0, 1.0
		for i := 0; i < n; i++ {
			v := env
			if float64(i) < attack {
				v *= float64(i) / attack
			}
			l.add(at+i, y*v)
			x, y = x*c-y*s, x*s+y*c
			env *= fall
		}
		return
	}
	for i := 0; i < n; i++ {
		fr := f * (1 + (glide-1)*float64(i)/float64(n))
		phase += 2 * math.Pi * fr / l.rate
		v := env
		if float64(i) < attack {
			v *= float64(i) / attack
		}
		l.add(at+i, math.Sin(phase)*v)
		env *= fall
	}
}

// burst adds a short soft puff of low passed noise.
func (l *layer) burst(at int, length, tone, amp float64) {
	n := int(length * l.rate)
	var a float64
	for i := 0; i < n; i++ {
		x := float64(i) / float64(n)
		env := math.Sin(math.Pi * x)
		env *= env
		a += tone * (l.rng.Float64()*2 - 1 - a)
		l.add(at+i, a*env*amp)
	}
}

func (l *layer) drips(perSec float64) {
	for _, at := range l.times(perSec / 4) {
		f := 900 + l.rng.Float64()*1100
		amp := 0.05 + l.rng.Float64()*0.04
		l.ping(at, f, 1.5, 0.03, 0.2, amp)
		// A softer echo off the cave walls.
		l.ping(at+int(0.19*l.rate), f, 1.5, 0.03, 0.2, amp*0.3)
		l.pause()
	}
}

func (l *layer) crickets(perSec float64) {
	for _, at := range l.times(perSec / 4) {
		f := 2600 + l.rng.Float64()*500
		pulses := 3 + l.rng.IntN(3)
		amp := 0.025 + l.rng.Float64()*0.02
		for p := 0; p < pulses; p++ {
			l.ping(at+int(float64(p)*0.06*l.rate), f, 1, 0.012, 0.05, amp)
		}
		l.pause()
	}
}

func (l *layer) birds(perSec float64) {
	for _, at := range l.times(perSec / 5) {
		f := 1800 + l.rng.Float64()*900
		notes := 2 + l.rng.IntN(4)
		glide := 0.8 + l.rng.Float64()*0.6
		amp := 0.025 + l.rng.Float64()*0.02
		for p := 0; p < notes; p++ {
			l.ping(at+int(float64(p)*0.11*l.rate), f*(1+0.06*float64(p%2)), glide, 0.04, 0.09, amp)
		}
		l.pause()
	}
}

func (l *layer) crackle(perSec float64) {
	for _, at := range l.times(perSec) {
		l.burst(at, 0.004+l.rng.Float64()*0.01, 0.25, 0.04+l.rng.Float64()*0.05)
	}
	l.pause()
}

func (l *layer) pages(perSec float64) {
	for _, at := range l.times(perSec / 5) {
		l.burst(at, 0.15+l.rng.Float64()*0.2, 0.12, 0.2)
		l.pause()
	}
}

// ticks adds a calm clock, a tick and a lower tock, a whole number of
// them to the loop.
func (l *layer) ticks() {
	secs := float64(l.n) / l.rate
	count := int(math.Round(secs / (0.8 + l.rng.Float64()*0.4)))
	count += count % 2 // keep the tick and tock in step round the loop
	step := float64(l.n) / float64(max(2, count))
	offset := l.rng.Float64() * step
	for k := 0; k < max(2, count); k++ {
		at := int(offset + float64(k)*step)
		f := 1500.0
		if k%2 == 1 {
			f = 1100
		}
		l.ping(at, f, 0.9, 0.006, 0.05, 0.1)
		l.burst(at, 0.01, 0.2, 0.05)
	}
}

// chimes rings soft bells on notes of the song's own scale.
func (l *layer) chimes(s Song, perSec float64) {
	scale := s.Scale
	if len(scale) == 0 {
		scale = minor
	}
	for _, at := range l.times(perSec / 5) {
		k := s.Root + 12 + scale[l.rng.IntN(len(scale))] + 12*l.rng.IntN(2)
		f := note(float64(k))
		for f > 1800 {
			f /= 2
		}
		amp := 0.04 + l.rng.Float64()*0.02
		l.ping(at, f, 1, 0.5, 1.8, amp)
		l.ping(at, f*2.01, 1, 0.25, 1.2, amp*0.25)
		l.pause()
	}
}
