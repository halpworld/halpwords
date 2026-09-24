package proc

import (
	"image/color"
	"testing"
)

func TestBrickWallDeterministic(t *testing.T) {
	ramp := []color.RGBA{{0, 0, 0, 255}, {50, 50, 50, 255}, {100, 100, 100, 255}, {200, 200, 200, 255}}
	a := BrickWall(64, 32, ramp, 42)
	b := BrickWall(64, 32, ramp, 42)
	c := BrickWall(64, 32, ramp, 43)
	same, diff := true, false
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			same = false
		}
		if a.Pix[i] != c.Pix[i] {
			diff = true
		}
	}
	if !same {
		t.Error("same seed gave different walls")
	}
	if !diff {
		t.Error("different seeds gave identical walls")
	}
}

func TestValueNoiseRange(t *testing.T) {
	for i := 0; i < 1000; i++ {
		v := ValueNoise(float64(i)*1.7, float64(i)*0.3, 5, 7)
		if v < 0 || v >= 1 {
			t.Fatalf("noise out of range: %v", v)
		}
	}
}

func TestFeatureSprites(t *testing.T) {
	opaque := func(m *Indexed) int {
		n := 0
		for _, p := range m.Pix {
			if p != Transparent {
				n++
			}
		}
		return n
	}
	for name, m := range map[string]*Indexed{
		"shrine":   ShrineSprite(0),
		"campfire": CampfireSprite(1, false),
		"embers":   CampfireSprite(1, true),
		"merchant": MerchantSprite(1),
	} {
		if opaque(m) < 60 {
			t.Errorf("%s: only %d pixels drawn", name, opaque(m))
		}
	}
	boss := MonsterSprite(0, 12, 1, 0)
	crowned := Crown(boss)
	if crowned.H <= boss.H || opaque(crowned) <= opaque(boss) {
		t.Fatal("the crown added nothing")
	}
}
