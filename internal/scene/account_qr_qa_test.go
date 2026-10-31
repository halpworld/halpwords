package scene

import (
	"strings"
	"testing"

	"github.com/halpworld/halpwords/assets"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/unifont"
)

// The widest "Is this you?" lines the cleaned names can make (a learner
// name as long as the website allows, and the longest school name, of the
// widest characters) still fit two lines of the dialog, and never draw past it.
func TestQAPairedWhoWorstCases(t *testing.T) {
	face, err := unifont.ParseBytes(assets.UnifontHex)
	if err != nil {
		t.Fatal(err)
	}
	f := gfx.NewFont(face)
	const tw = 480 - 40
	names := []string{
		strings.Repeat("W", 40), strings.Repeat("漢", 40), strings.Repeat("🎓", 40), strings.Repeat("é", 40),
		strings.Repeat("ＷＷ", 20), "", "x",
	}
	for _, name := range []string{"", "Aoife", strings.Repeat("W", 30), strings.Repeat("漢", 30), strings.Repeat("Ó", 60)} {
		for _, w := range names {
			s := pairedWho(name, w)
			lines := wrap2(f, s, tw)
			if len(lines) > 2 {
				t.Errorf("%q wraps to %d lines", s, len(lines))
			}
			for _, l := range lines {
				if px := f.Width(l, 1); px > tw {
					t.Errorf("%.30q... line is %dpx wide, room for %d", l, px, tw)
				}
			}
		}
	}
	// No panic or odd output for odd inputs.
	for _, tc := range [][2]string{{"", ""}, {"", "family"}, {"A", "\x00"}, {"‮", "‮"}} {
		if s := pairedWho(tc[0], tc[1]); s == "" {
			t.Errorf("pairedWho(%q, %q) is empty", tc[0], tc[1])
		}
	}
}
