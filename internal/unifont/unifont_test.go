package unifont

import (
	"testing"

	"github.com/halpworld/halpwords/assets"
)

func TestParseEmbedded(t *testing.T) {
	f, err := ParseBytes(assets.UnifontHex)
	if err != nil {
		t.Fatal(err)
	}
	// Every character the four starter languages need must be present.
	need := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"éèêëàâçîïôùûüœÉÈÇŒ" + // French
		"āēīōūȳĀĒĪŌŪ" + // Latin macrons
		"áéíóúÁÉÍÓÚ" + // Irish fadas
		"αβγδεζηθικλμνξοπρσςτυφχψω" + // Greek
		"ἀἁἄἅἂἃἆἇᾀᾳᾷῥὠὡὤὥῶῷάέήίόύώϊϋΐΰ" + // polytonic
		"◄►▲▼♥★"
	for _, r := range need {
		if !f.Has(r) {
			t.Errorf("missing glyph %q (U+%04X)", r, r)
		}
	}
}

func TestGlyphBits(t *testing.T) {
	f, err := ParseBytes([]byte("0041:0000000018242442427E424242420000\n"))
	if err != nil {
		t.Fatal(err)
	}
	g := f.Glyph('A')
	if g.Width != 8 {
		t.Fatalf("width = %d", g.Width)
	}
	// Row 4 is 0x18 = 00011000.
	for x := 0; x < 8; x++ {
		want := x == 3 || x == 4
		if g.Set(x, 4) != want {
			t.Errorf("pixel (%d,4) = %v, want %v", x, g.Set(x, 4), want)
		}
	}
}
