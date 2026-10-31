package audio

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/halpworld/halpwords/pkg/proc"
)

// worldMoods are the moods a world shapes.
var worldMoods = []Mood{Delve, Fight, Boss}

func TestEveryThemeHasItsOwnWorld(t *testing.T) {
	if len(Worlds) != len(proc.Themes) {
		t.Errorf("%d worlds with music for %d themes", len(Worlds), len(proc.Themes))
	}
	seen := map[World]string{}
	for _, th := range proc.Themes {
		w := WorldNamed(th.Name)
		if w == NoWorld {
			t.Errorf("%q has no music of its own", th.Name)
			continue
		}
		if o, ok := seen[w]; ok {
			t.Errorf("%q and %q share %s", th.Name, o, w)
		}
		seen[w] = th.Name
		// A remix on a later lap keeps the name, and so the music.
		if r := proc.World(slices.IndexFunc(proc.Themes, func(x proc.Theme) bool { return x.Name == th.Name }), 1); WorldNamed(r.Name) != w {
			t.Errorf("%q remixed plays %s", th.Name, WorldNamed(r.Name))
		}
	}
	if WorldNamed("") != NoWorld || WorldNamed("Nowhere") != NoWorld {
		t.Error("an unknown name has a world")
	}
	if NoWorld.String() != "none" || World(200).String() != "unknown" {
		t.Error("bad names for NoWorld or out of range")
	}
	names := map[string]bool{}
	for _, w := range Worlds {
		if n := w.String(); n == "none" || n == "unknown" || names[n] {
			t.Errorf("world %d is called %q", w, n)
		} else {
			names[n] = true
		}
	}
}

func TestWorldsSoundDistinct(t *testing.T) {
	keys := map[string]World{}
	for _, w := range Worlds {
		s := Compose(Track{Mood: Delve, Seed: 1, World: w})
		key := fmt.Sprint(s.Root%12, s.Scale)
		if o, ok := keys[key]; ok {
			t.Errorf("%s and %s are in the same key and scale, %s", w, o, key)
		}
		keys[key] = w
		p := palettes[w]
		if len(p.scale) < 5 || len(p.progs) == 0 || p.bpm[0] < 72 || p.bpm[1] < p.bpm[0] || p.leadV <= 0 || p.busy <= 0 {
			t.Errorf("%s: an incomplete palette %+v", w, p)
		}
	}
}

func TestWorldFixesKeyNotTune(t *testing.T) {
	for _, w := range Worlds {
		for _, m := range worldMoods {
			a := Compose(Track{Mood: m, Seed: 4, World: w})
			b := Compose(Track{Mood: m, Seed: 5, World: w})
			if a.Root != b.Root || !slices.Equal(a.Scale, b.Scale) {
				t.Errorf("%s/%s: the key moves from floor to floor", w, m)
			}
			if slices.Equal(a.Notes, b.Notes) {
				t.Errorf("%s/%s: two floors have the same tune", w, m)
			}
			if m == Delve {
				p := palettes[w]
				if a.BPM < p.bpm[0] || a.BPM > p.bpm[1] {
					t.Errorf("%s: %.1f bpm outside %v", w, a.BPM, p.bpm)
				}
			}
			if plain := Compose(Track{Mood: m, Seed: 4}); slices.Equal(a.Notes, plain.Notes) {
				t.Errorf("%s/%s: the world changes nothing", w, m)
			}
		}
	}
}

func TestWorldMusicIsValid(t *testing.T) {
	for _, w := range Worlds {
		for _, m := range worldMoods {
			for seed := uint64(0); seed < 8; seed++ {
				s := Compose(Track{Mood: m, Seed: seed, World: w})
				name := fmt.Sprintf("%s/%s/%d", w, m, seed)
				if len(s.Notes) < 20 {
					t.Fatalf("%s: only %d notes", name, len(s.Notes))
				}
				if l := s.Loop(); l < 4 || l > 30 {
					t.Errorf("%s: a %.1fs loop", name, l)
				}
				in := map[int]bool{}
				for _, k := range s.Scale {
					in[k] = true
				}
				for i, n := range s.Notes {
					if n.Delay < 0 || n.Delay >= s.Loop() {
						t.Fatalf("%s: note %d starts at %.2fs, outside the loop", name, i, n.Delay)
					}
					if n.Attack <= 0 || n.Release <= 0 || n.Volume <= 0 || n.Volume > 1 {
						t.Fatalf("%s: note %d has a bad envelope: %+v", name, i, n)
					}
					if top := 5000.0; n.Freq < 20 || n.Freq > top && (n.Wave != Noise || n.Freq > 12000) {
						t.Fatalf("%s: note %d at %.0f Hz", name, i, n.Freq)
					}
					if n.Wave == Noise || n.Slide != 0 {
						continue // drums
					}
					k := int(math.Round(12 * math.Log2(n.Freq/440)))
					if !in[((k-s.Root)%12+12)%12] {
						t.Fatalf("%s: a note %d semitones from A4 is not in the key of %d %v", name, k, s.Root, s.Scale)
					}
				}
			}
		}
	}
}

func TestOtherMoodsIgnoreWorld(t *testing.T) {
	for _, m := range []Mood{Title, Camp, Lament} {
		a := Compose(Track{Mood: m, Seed: 2})
		b := Compose(Track{Mood: m, Seed: 2, World: Forge})
		if !slices.Equal(a.Notes, b.Notes) || a.BPM != b.BPM {
			t.Errorf("%s: the world changed the music", m)
		}
	}
	if s := Compose(Track{Mood: Quiet, World: Ice}); len(s.Notes) != 0 {
		t.Error("quiet plays in the Ice Halls")
	}
}

func TestWorldMusicIsDeterministic(t *testing.T) {
	for _, w := range Worlds {
		tr := Track{Mood: Delve, Seed: 42, World: w, Ambience: worldAmb[w]}
		a, b := Compose(tr), Compose(tr)
		if !slices.Equal(a.Notes, b.Notes) || a.BPM != b.BPM || a.Root != b.Root {
			t.Fatalf("%s: the same track composed twice differs", w)
		}
		if !slices.Equal(RenderLoop(a, SampleRate, nil), RenderLoop(b, SampleRate, nil)) {
			t.Fatalf("%s: the same track rendered twice differs", w)
		}
	}
}

// worldAmb is each world's ambience, as the game pairs them.
var worldAmb = map[World]Ambience{
	Crypt: Dust, Cellars: Crickets, Caves: Drips, Forge: Rumble, Ice: Wind,
	Library: Pages, Garden: Birds, Workshop: Ticking, Vaults: Chimes, Tomb: Sand,
}

func rms(s []float32) float64 {
	sum := 0.0
	for _, v := range s {
		sum += float64(v) * float64(v)
	}
	return math.Sqrt(sum / float64(len(s)))
}

// TestWorldLoopsAreCleanAndEven checks every world's loops for a seam and
// clipping, and that no world is much louder or quieter than the plain
// Delve music, so moving between worlds doesn't jump in volume.
func TestWorldLoopsAreCleanAndEven(t *testing.T) {
	plain := rms(RenderLoop(Compose(Track{Mood: Delve, Seed: 9}), SampleRate, nil))
	for _, w := range Worlds {
		for _, m := range worldMoods {
			tr := Track{Mood: m, Seed: 9, World: w}
			if m == Delve {
				tr.Ambience = worldAmb[w]
			}
			s := Compose(tr)
			out := RenderLoop(s, SampleRate, nil)
			if want := int(math.Round(s.Loop() * SampleRate)); len(out) != want {
				t.Fatalf("%s/%s: %d samples, want %d", w, m, len(out), want)
			}
			step := 0.0
			for i, v := range out {
				if v < -1 || v > 1 || math.IsNaN(float64(v)) {
					t.Fatalf("%s/%s: sample %d is %v", w, m, i, v)
				}
				if i > 0 {
					step = max(step, math.Abs(float64(v-out[i-1])))
				}
			}
			if seam := math.Abs(float64(out[0] - out[len(out)-1])); seam > step*1.5+0.002 {
				t.Errorf("%s/%s: seam of %.4f, biggest step %.4f", w, m, seam, step)
			}
			if p := peakOf(out); p > 0.98 || p < 0.3 {
				t.Errorf("%s/%s: peak %.3f", w, m, p)
			}
			if m != Delve {
				continue
			}
			if r := rms(out); r < plain*0.5 || r > plain*2 {
				t.Errorf("%s: rms %.3f against plain Delve %.3f", w, r, plain)
			}
			// The ambience stays under this world's music too.
			if amb := peakOf(RenderAmbience(s, SampleRate, nil)); amb > 0.2*peakOf(RenderLoop(Compose(Track{Mood: Delve, Seed: 9, World: w}), SampleRate, nil)) {
				t.Errorf("%s: ambience peak %.3f is too loud", w, amb)
			}
		}
	}
}

func BenchmarkRenderLoopWorld(b *testing.B) {
	for _, w := range Worlds {
		b.Run(w.String(), func(b *testing.B) {
			s := Compose(Track{Mood: Delve, Seed: 1, World: w, Ambience: worldAmb[w]})
			for b.Loop() {
				RenderLoop(s, SampleRate, nil)
			}
		})
	}
}

// toneSeconds is how many seconds of tones a song renders: with the loop
// length, what a render costs in time and memory.
func toneSeconds(s Song) float64 {
	c := 0.0
	for _, n := range s.Notes {
		c += n.Attack + n.Hold + n.Release
	}
	return c
}

// TestWorldMusicCostsAboutTheSame keeps the worlds within reach of the
// plain Delve music, which the web build already renders in the
// background: loops no longer than 27 seconds (the slowest plain loop is
// about 25) and at most 1.7 times the tones (pads hold three notes a bar;
// steady notes are the cheapest to render).
func TestWorldMusicCostsAboutTheSame(t *testing.T) {
	plain := 0.0
	for seed := uint64(0); seed < 10; seed++ {
		plain += toneSeconds(Compose(Track{Mood: Delve, Seed: seed}))
	}
	plain /= 10
	for _, w := range Worlds {
		for seed := uint64(0); seed < 10; seed++ {
			s := Compose(Track{Mood: Delve, Seed: seed, World: w})
			if s.Loop() > 27 {
				t.Errorf("%s/%d: a %.1fs loop", w, seed, s.Loop())
			}
			if c := toneSeconds(s); c > 1.7*plain {
				t.Errorf("%s/%d: %.0fs of tones against %.0fs plain", w, seed, c, plain)
			}
		}
	}
}
