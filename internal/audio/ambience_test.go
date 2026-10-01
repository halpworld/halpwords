package audio

import (
	"math"
	"slices"
	"testing"
)

func peakOf(s []float32) float64 {
	p := 0.0
	for _, v := range s {
		p = max(p, math.Abs(float64(v)))
	}
	return p
}

func TestAmbienceNames(t *testing.T) {
	seen := map[string]bool{}
	for _, a := range Ambiences {
		n := a.String()
		if n == "unknown" || n == "none" || seen[n] {
			t.Errorf("ambience %d has name %q", a, n)
		}
		seen[n] = true
	}
	if None.String() != "none" || Ambience(200).String() != "unknown" {
		t.Error("bad names for None or out of range")
	}
}

func TestAmbienceIsDeterministic(t *testing.T) {
	for _, a := range Ambiences {
		s := Compose(Track{Mood: Delve, Seed: 5, Ambience: a})
		x, y := RenderAmbience(s, SampleRate, nil), RenderAmbience(s, SampleRate, nil)
		if len(x) == 0 || !slices.Equal(x, y) {
			t.Errorf("%s: renders differ or are empty", a)
		}
		o := Compose(Track{Mood: Delve, Seed: 6, Ambience: a})
		if slices.Equal(x, RenderAmbience(o, SampleRate, nil)) {
			t.Errorf("%s: seeds 5 and 6 sound the same", a)
		}
	}
}

func TestAmbienceLoopsWithoutASeam(t *testing.T) {
	for _, a := range Ambiences {
		s := Compose(Track{Mood: Delve, Seed: 9, Ambience: a})
		for _, out := range [][]float32{RenderAmbience(s, SampleRate, nil), RenderLoop(s, SampleRate, nil)} {
			// The jump from the last sample to the first is no bigger
			// than the biggest step inside the loop.
			step := 0.0
			for i := 1; i < len(out); i++ {
				step = max(step, math.Abs(float64(out[i]-out[i-1])))
			}
			seam := math.Abs(float64(out[0] - out[len(out)-1]))
			if seam > step*1.5+0.002 {
				t.Errorf("%s: seam of %.4f, biggest step %.4f", a, seam, step)
			}
		}
	}
}

func TestAmbienceIsQuiet(t *testing.T) {
	for _, a := range Ambiences {
		for seed := uint64(1); seed <= 4; seed++ {
			s := Compose(Track{Mood: Delve, Seed: seed, Ambience: a})
			music := peakOf(RenderLoop(Compose(Track{Mood: Delve, Seed: seed}), SampleRate, nil))
			amb := RenderAmbience(s, SampleRate, nil)
			p := peakOf(amb)
			if p < 0.01 {
				t.Errorf("%s/%d: inaudible, peak %.3f", a, seed, p)
			}
			if p > 0.2*music {
				t.Errorf("%s/%d: peak %.3f against music %.3f", a, seed, p, music)
			}
			mixed := RenderLoop(s, SampleRate, nil)
			if peakOf(mixed) > 1 {
				t.Errorf("%s/%d: mixed loop clips", a, seed)
			}
		}
	}
}

func TestNoAmbienceIsTodaysMusic(t *testing.T) {
	a := RenderLoop(Compose(Track{Mood: Delve, Seed: 3}), SampleRate, nil)
	b := RenderLoop(Compose(Track{Mood: Delve, Seed: 3, Ambience: None}), SampleRate, nil)
	if !slices.Equal(a, b) {
		t.Fatal("None changes the music")
	}
	// The music itself is the same under any ambience; only a layer is added.
	s := Compose(Track{Mood: Delve, Seed: 3, Ambience: Drips})
	if !slices.Equal(s.Notes, Compose(Track{Mood: Delve, Seed: 3}).Notes) {
		t.Error("an ambience changes the notes")
	}
	if slices.Equal(a, RenderLoop(s, SampleRate, nil)) {
		t.Error("an ambience adds nothing")
	}
}

func TestOtherMoodsIgnoreAmbience(t *testing.T) {
	for _, m := range Moods {
		if m == Delve {
			continue
		}
		a := RenderLoop(Compose(Track{Mood: m, Seed: 2}), SampleRate, nil)
		b := RenderLoop(Compose(Track{Mood: m, Seed: 2, Ambience: Birds}), SampleRate, nil)
		if !slices.Equal(a, b) {
			t.Errorf("%s: ambience changed the music", m)
		}
	}
}

func TestTracksWithDifferentAmbienceDiffer(t *testing.T) {
	a, b := Track{Mood: Delve, Seed: 1}, Track{Mood: Delve, Seed: 1, Ambience: Wind}
	if a == b {
		t.Fatal("tracks with different ambience are equal")
	}
	m := map[Track]bool{a: true}
	if m[b] || !m[Track{Mood: Delve, Seed: 1}] {
		t.Error("tracks do not work as map keys")
	}
}

func BenchmarkRenderLoopDelve(b *testing.B) {
	s := Compose(Track{Mood: Delve, Seed: 1})
	for b.Loop() {
		RenderLoop(s, SampleRate, nil)
	}
}

func BenchmarkRenderLoopDelveAmbience(b *testing.B) {
	for _, a := range Ambiences {
		b.Run(a.String(), func(b *testing.B) {
			s := Compose(Track{Mood: Delve, Seed: 1, Ambience: a})
			for b.Loop() {
				RenderLoop(s, SampleRate, nil)
			}
		})
	}
}
