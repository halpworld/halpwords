package scene

import (
	"fmt"
	"strings"
	"testing"

	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/pkg/proc"
)

func TestShowLongCode(t *testing.T) {
	for _, c := range []struct {
		code string
		n    int
		want string
	}{
		{"", link.CardCodeLen, "____-____-____"},
		{"ABCDEF", link.CardCodeLen, "ABCD-EF__-____"},
		{"C1A55C0D", link.ClassCodeLen, "C1A5-5C0D"},
	} {
		if got := showLongCode([]rune(c.code), c.n); got != c.want {
			t.Errorf("showLongCode(%q, %d) = %q, want %q", c.code, c.n, got, c.want)
		}
	}
}

// Every picture the server may send has an image to show.
func TestPictureImages(t *testing.T) {
	for i := range proc.PictureCount {
		if pictureImage(i) == nil {
			t.Errorf("no image for picture %d", i)
		}
	}
	if pictureImage(proc.PictureCount) != nil || pictureImage(-1) != nil {
		t.Error("an image for a picture that doesn't exist")
	}
}

// The picture keys on the keypad are the numbers on the labels, not the
// keypad's layout (#47).
func TestPictureKeysMatchLabels(t *testing.T) {
	for i, keys := range pictureKeys {
		want := fmt.Sprint(i + 1)
		for _, k := range keys {
			if got := k.String(); !strings.HasSuffix(got, want) {
				t.Errorf("picture %s has key %v", want, got)
			}
		}
	}
}

// A letter in the class's name list only jumps: W and S are also Up and
// Down in menus, and must not move the selection again (#46).
func TestNameListLetterOnlyJumps(t *testing.T) {
	names := []string{"Aoife", "Brian", "Sam", "Will", "Zoe"}
	if got := jumpName(names, 0, []rune("s")); got != 2 {
		t.Errorf("s: %d, want 2 (Sam)", got)
	}
	if got := jumpName(names, 0, []rune("W")); got != 3 {
		t.Errorf("W: %d, want 3 (Will)", got)
	}
	if got := jumpName(names, 3, []rune("q")); got != 3 {
		t.Errorf("no name with q moved to %d", got)
	}
}
