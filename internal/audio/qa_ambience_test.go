package audio

import (
	"math"
	"slices"
	"testing"
)

func TestQAAmbienceQuiet(t *testing.T) {
	for _, a := range Ambiences {
		for _, seed := range []uint64{0, 1, 2, 42, 1 << 40} {
			s := Compose(Track{Mood: Delve, Seed: seed, Ambience: a})
			amb := RenderAmbience(s, SampleRate, nil)
			if len(amb) == 0 {
				t.Fatalf("%s seed %d: empty", a, seed)
			}
			sum := 0.0
			for _, v := range amb {
				if math.IsNaN(float64(v)) {
					t.Fatalf("%s seed %d: NaN", a, seed)
				}
				sum += float64(v) * float64(v)
			}
			rms := math.Sqrt(sum / float64(len(amb)))
			if p := peakOf(amb); p > ambienceCap+1e-6 || rms > 0.05 {
				t.Errorf("%s seed %d: peak %.4f rms %.4f", a, seed, p, rms)
			}
			if rms == 0 {
				t.Errorf("%s seed %d: silent", a, seed)
			}
			mixed := RenderLoop(s, SampleRate, nil)
			if p := peakOf(mixed); p > 1 {
				t.Errorf("%s seed %d: mixed peak %.4f", a, seed, p)
			}
		}
	}
}

func TestQAAmbienceLoopLengthAndMusicUnchanged(t *testing.T) {
	for _, a := range Ambiences {
		base := Compose(Track{Mood: Delve, Seed: 11})
		s := Compose(Track{Mood: Delve, Seed: 11, Ambience: a})
		x, y := RenderLoop(base, SampleRate, nil), RenderLoop(s, SampleRate, nil)
		if len(x) != len(y) {
			t.Errorf("%s: loop %d samples vs %d without", a, len(y), len(x))
		}
		if slices.Equal(x, y) {
			t.Errorf("%s: mix identical to no ambience", a)
		}
		if !slices.Equal(RenderLoop(s, SampleRate, nil), y) {
			t.Errorf("%s: RenderLoop not deterministic", a)
		}
	}
	if RenderAmbience(Compose(Track{Mood: Delve, Seed: 1}), SampleRate, nil) != nil {
		t.Error("None should render nothing")
	}
}

func TestQAAmbienceYields(t *testing.T) {
	for _, a := range Ambiences {
		s := Compose(Track{Mood: Delve, Seed: 3, Ambience: a})
		n := 0
		RenderAmbience(s, SampleRate, func() { n++ })
		// Ticking is cheap (under 2ms native) and never yields; the rest do.
		if n < 1 && a != Ticking {
			t.Errorf("%s: only %d yields in the ambience render", a, n)
		}
		total := 0
		RenderLoop(s, SampleRate, func() { total++ })
		none := 0
		RenderLoop(Compose(Track{Mood: Delve, Seed: 3}), SampleRate, func() { none++ })
		if total < none || (a != Ticking && total <= none) {
			t.Errorf("%s: RenderLoop yields %d with ambience, %d without", a, total, none)
		}
	}
}
