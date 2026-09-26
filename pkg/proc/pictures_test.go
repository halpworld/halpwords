package proc

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

func TestPictures(t *testing.T) {
	all := Pictures()
	if len(all) != PictureCount {
		t.Fatalf("%d pictures, want %d", len(all), PictureCount)
	}
	slugs := map[string]bool{}
	shapes := map[string]string{}
	for i, p := range all {
		if slugs[p.Slug] {
			t.Errorf("%s: slug used twice", p.Slug)
		}
		slugs[p.Slug] = true
		filled := 0
		for y, r := range p.rows {
			if len(r) != PictureSize || strings.Trim(r, ".#+") != "" {
				t.Errorf("%s: row %d is %q", p.Slug, y, r)
			}
			filled += strings.Count(r, "#") + strings.Count(r, "+")
		}
		if filled < 12 {
			t.Errorf("%s: only %d pixels", p.Slug, filled)
		}
		key := strings.Join(p.rows[:], "/")
		if other, dup := shapes[key]; dup {
			t.Errorf("%s looks like %s", p.Slug, other)
		}
		shapes[key] = p.Slug
		if got, ok := PictureAt(i); !ok || got.Slug != p.Slug {
			t.Errorf("PictureAt(%d) = %v", i, got.Slug)
		}
		n := 0
		for _, r := range p.Runs() {
			n += r.W
		}
		if n != filled {
			t.Errorf("%s: runs cover %d pixels, want %d", p.Slug, n, filled)
		}
		img := p.Image(3)
		if b := img.Bounds(); b.Dx() != 3*PictureSize || b.Dy() != 3*PictureSize {
			t.Errorf("%s: image is %v", p.Slug, b)
		}
		for _, r := range p.Runs() {
			if got := img.RGBAAt(r.X*3+1, r.Y*3+1); got != r.Colour {
				t.Errorf("%s: image pixel %d,%d is %v, want %v", p.Slug, r.X, r.Y, got, r.Colour)
			}
		}
	}
	if _, ok := PictureAt(PictureCount); ok {
		t.Error("PictureAt(PictureCount) found a picture")
	}
	if _, ok := PictureAt(-1); ok {
		t.Error("PictureAt(-1) found a picture")
	}
}

// TestPicturesNeverChange pins the set: picture passwords stored on the
// server name pictures by index, so a picture may never move, change its
// look or go. Adding one at the end changes the fingerprint; update it
// then, and only then.
func TestPicturesNeverChange(t *testing.T) {
	h := sha256.New()
	for _, p := range Pictures() {
		fmt.Fprintf(h, "%s %v %v %s\n", p.Slug, p.Main, p.Accent, strings.Join(p.rows[:], "/"))
	}
	const want = "0de61ecc84cb7084cf8fec6cb0058d9fcf55fe27a1d89e6254c7690739824aa7"
	if got := fmt.Sprintf("%x", h.Sum(nil)); got != want {
		t.Errorf("the picture set changed: fingerprint %s, want %s", got, want)
	}
}
