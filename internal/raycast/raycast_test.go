package raycast

import (
	"testing"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/proc"
)

func startCamera(l *dungeon.Level) Camera {
	d := l.StartDir.Delta()
	return NewCamera(float64(l.Start.X)+0.5-0.45*float64(d.X), float64(l.Start.Y)+0.5-0.45*float64(d.Y), Angle(l.StartDir), 0.75)
}

func TestRender(t *testing.T) {
	for seed := uint64(1); seed <= 20; seed++ {
		l := dungeon.Generate(seed, 1+int(seed%5))
		tex := NewTextures(proc.ThemeFor(l.Depth), seed)
		r := New(192, 120)
		var sprites []Sprite
		for _, m := range l.Monsters {
			sprites = append(sprites, Sprite{X: float64(m.At.X) + 0.5, Y: float64(m.At.Y) + 0.5, Size: m.Kind.Size, Img: proc.MonsterSprite(m.Kind.Family, m.Kind.Hue, m.Seed, 0)})
		}
		r.Render(l, tex, startCamera(l), sprites, 0)
		lit := 0
		for i := 0; i < len(r.Img.Pix); i += 4 {
			if r.Img.Pix[i+3] != 0xff {
				t.Fatalf("seed %d: pixel %d not drawn", seed, i/4)
			}
			if r.Img.Pix[i]|r.Img.Pix[i+1]|r.Img.Pix[i+2] != 0 {
				lit++
			}
		}
		if lit < len(r.Img.Pix)/4/3 {
			t.Fatalf("seed %d: view is mostly black (%d lit)", seed, lit)
		}
		if !l.Seen[l.Index(l.Start.Step(l.StartDir))] && !l.At(l.Start.Step(l.StartDir)).Solid() {
			t.Fatalf("seed %d: cell ahead not marked seen", seed)
		}
		for x, z := range r.zbuf {
			if z <= 0 || z > 64 {
				t.Fatalf("seed %d: bad depth %v at column %d", seed, z, x)
			}
		}
	}
}

func TestSpriteOcclusion(t *testing.T) {
	l := dungeon.Generate(3, 1)
	tex := NewTextures(proc.ThemeFor(1), 1)
	r := New(64, 40)
	cam := startCamera(l)
	img := proc.NewIndexed(4, 4)
	img.Rect(0, 0, 4, 4, proc.Themes[0].Wall[5])
	// A sprite far behind the camera must not be drawn; one in front must be.
	behind := Sprite{X: cam.X - cam.DirX*3, Y: cam.Y - cam.DirY*3, Size: 1, Img: img, Flash: true}
	r.Render(l, tex, cam, []Sprite{behind}, 0)
	if countWhite(r) != 0 {
		t.Fatal("sprite behind the camera was drawn")
	}
	front := Sprite{X: cam.X + cam.DirX*0.6, Y: cam.Y + cam.DirY*0.6, Size: 0.5, Img: img, Flash: true}
	r.Render(l, tex, cam, []Sprite{front}, 0)
	if countWhite(r) == 0 {
		t.Fatal("sprite in front of the camera was not drawn")
	}
}

func countWhite(r *Renderer) int {
	n := 0
	for i := 0; i < len(r.Img.Pix); i += 4 {
		if r.Img.Pix[i] == 0xff && r.Img.Pix[i+1] == 0xff && r.Img.Pix[i+2] == 0xff {
			n++
		}
	}
	return n
}

func BenchmarkRender(b *testing.B) {
	l := dungeon.Generate(1, 3)
	tex := NewTextures(proc.ThemeFor(1), 1)
	r := New(192, 120)
	cam := startCamera(l)
	for i := 0; i < b.N; i++ {
		r.Render(l, tex, cam, nil, uint64(i))
	}
}
