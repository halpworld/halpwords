package audio

import (
	"math"
	"slices"
	"testing"
)

func TestEveryMoodComposes(t *testing.T) {
	for _, m := range Moods {
		for seed := uint64(0); seed < 20; seed++ {
			s := Compose(Track{m, seed})
			if len(s.Notes) < 20 {
				t.Fatalf("%s/%d: only %d notes", m, seed, len(s.Notes))
			}
			if l := s.Loop(); l < 4 || l > 30 {
				t.Errorf("%s/%d: a %.1fs loop", m, seed, l)
			}
			for i, n := range s.Notes {
				if n.Delay < 0 || n.Delay >= s.Loop() {
					t.Fatalf("%s/%d: note %d starts at %.2fs, outside the loop", m, seed, i, n.Delay)
				}
				if n.Attack <= 0 || n.Release <= 0 || n.Volume <= 0 || n.Volume > 1 {
					t.Fatalf("%s/%d: note %d has a bad envelope: %+v", m, seed, i, n)
				}
				if n.Freq < 20 || n.Freq > 12000 {
					t.Fatalf("%s/%d: note %d at %.0f Hz", m, seed, i, n.Freq)
				}
			}
		}
	}
}

func TestMelodyStaysInKey(t *testing.T) {
	for _, m := range Moods {
		s := Compose(Track{m, 7})
		in := map[int]bool{}
		for _, k := range s.Scale {
			in[k] = true
		}
		for _, n := range s.Notes {
			if n.Wave == Noise || n.Slide != 0 {
				continue // drums
			}
			k := int(math.Round(12 * math.Log2(n.Freq/440)))
			if !in[((k-s.Root)%12+12)%12] {
				t.Fatalf("%s: a note %d semitones from A4 is not in the key of %d %v", m, k, s.Root, s.Scale)
			}
		}
	}
}

func TestComposeIsDeterministic(t *testing.T) {
	a, b := Compose(Track{Fight, 42}), Compose(Track{Fight, 42})
	if !slices.Equal(a.Notes, b.Notes) || a.BPM != b.BPM {
		t.Fatal("the same track composed twice differs")
	}
	if c := Compose(Track{Fight, 43}); slices.Equal(a.Notes, c.Notes) {
		t.Fatal("different seeds give the same tune")
	}
	if c := Compose(Track{Delve, 42}); c.BPM >= a.BPM {
		t.Errorf("exploring (%.0f bpm) should be slower than fighting (%.0f bpm)", c.BPM, a.BPM)
	}
}

func TestRenderLoop(t *testing.T) {
	for _, m := range Moods {
		s := Compose(Track{m, 3})
		yields := 0
		out := RenderLoop(s, SampleRate, func() { yields++ })
		if want := int(math.Round(s.Loop() * SampleRate)); len(out) != want {
			t.Fatalf("%s: %d samples, want %d", m, len(out), want)
		}
		if yields == 0 {
			t.Errorf("%s: the render never yielded", m)
		}
		loud := 0.0
		for i, v := range out {
			if v < -1 || v > 1 || math.IsNaN(float64(v)) {
				t.Fatalf("%s: sample %d is %v", m, i, v)
			}
			loud = max(loud, math.Abs(float64(v)))
		}
		if loud < 0.1 {
			t.Errorf("%s: nearly silent, peak %.2f", m, loud)
		}
	}
}

func TestQuietIsSilent(t *testing.T) {
	if s := Compose(Track{Quiet, 1}); len(s.Notes) != 0 {
		t.Fatalf("quiet has %d notes", len(s.Notes))
	}
}
