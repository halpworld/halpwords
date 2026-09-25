package proc

import "testing"

func TestIcon(t *testing.T) {
	img := Icon()
	if b := img.Bounds(); b.Dx() != IconSize || b.Dy() != IconSize {
		t.Fatalf("icon is %v", b)
	}
	if img.RGBAAt(0, 0).A != 0 || img.RGBAAt(IconSize-1, IconSize-1).A != 0 {
		t.Error("the corners should be clear, for a rounded icon")
	}
	if img.RGBAAt(IconSize/2, IconSize/2).A != 255 {
		t.Error("the middle should be solid")
	}
	for _, size := range []int{16, 32, 64, 256} {
		if got := IconAt(size).Bounds().Dx(); got != size {
			t.Errorf("IconAt(%d) is %d wide", size, got)
		}
	}
}
