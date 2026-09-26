package scene

import (
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
