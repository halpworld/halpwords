package audio

import (
	"math"
	"slices"
	"testing"
)

func TestEverySoundIsDefined(t *testing.T) {
	seen := map[string]bool{}
	for id := ID(0); id < Count; id++ {
		name := id.String()
		if name == "" || seen[name] {
			t.Errorf("sound %d has a missing or repeated name %q", id, name)
		}
		seen[name] = true
		s := Sounds[id]
		if len(s) == 0 {
			t.Errorf("%s: no tones", name)
			continue
		}
		if l := s.Len(); l <= 0 || l > 2 {
			t.Errorf("%s: %.2fs long, want a short effect", name, l)
		}
		for i, tone := range s {
			// A fade in and out on every tone stops clicks.
			if tone.Attack <= 0 || tone.Release <= 0 {
				t.Errorf("%s tone %d: needs an attack and a release", name, i)
			}
			if tone.Volume <= 0 || tone.Volume > 1 {
				t.Errorf("%s tone %d: volume %v out of range", name, i, tone.Volume)
			}
		}
	}
}

func TestRenderStaysInRange(t *testing.T) {
	for id := ID(0); id < Count; id++ {
		out := Render(Sounds[id], SampleRate)
		if want := int(math.Ceil(Sounds[id].Len() * SampleRate)); len(out) != want {
			t.Errorf("%s: %d samples, want %d", id, len(out), want)
		}
		loud := false
		for i, v := range out {
			if v < -1 || v > 1 || math.IsNaN(float64(v)) {
				t.Fatalf("%s: sample %d is %v", id, i, v)
			}
			loud = loud || math.Abs(float64(v)) > 0.05
		}
		if !loud {
			t.Errorf("%s: silent", id)
		}
		// The sound fades out to nothing.
		if last := out[len(out)-1]; math.Abs(float64(last)) > 0.02 {
			t.Errorf("%s: ends on %v, want silence", id, last)
		}
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	for _, id := range []ID{Hit, Unseal, Door} {
		if !slices.Equal(Render(Sounds[id], SampleRate), Render(Sounds[id], SampleRate)) {
			t.Errorf("%s: two renders differ", id)
		}
	}
}

func TestSlideChangesPitch(t *testing.T) {
	// Count zero crossings in the first and last quarter of a falling tone.
	s := Sound{{Wave: Sine, Freq: 2000, Slide: -2, Attack: 0.001, Hold: 1, Release: 0.001, Volume: 1}}
	out := Render(s, SampleRate)
	crossings := func(v []float32) int {
		n := 0
		for i := 1; i < len(v); i++ {
			if (v[i-1] < 0) != (v[i] < 0) {
				n++
			}
		}
		return n
	}
	q := len(out) / 4
	if first, last := crossings(out[:q]), crossings(out[3*q:]); last*2 > first {
		t.Errorf("pitch did not fall: %d crossings at the start, %d at the end", first, last)
	}
}

func TestEncode(t *testing.T) {
	b := Encode([]float32{0, 1, -1})
	if len(b) != 12 {
		t.Fatalf("got %d bytes, want 12", len(b))
	}
	// 1.0 is 0x3f800000, little-endian.
	if b[4] != 0 || b[5] != 0 || b[6] != 0x80 || b[7] != 0x3f {
		t.Errorf("1.0 encoded as % x", b[4:8])
	}
}
