package raycast

import (
	"math"
	"reflect"
	"slices"
	"testing"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/pkg/proc"
)

// Building every world's textures, props and particles between level
// generations leaves the levels exactly as without (no shared RNG).
func TestQADeterminismAcrossVisuals(t *testing.T) {
	const seed = 1234
	plain := make([]*dungeon.Level, 26)
	for d := 1; d <= 25; d++ {
		plain[d] = dungeon.Generate(seed, d)
	}
	r := New(96, 60)
	for d := 1; d <= 25; d++ {
		th := proc.ThemeFor(d)
		pre := dungeon.Generate(seed, d)
		tex := NewTextures(th, pre.Seed)
		Decor(pre, th)
		r.SetWorld(th)
		cam := startCamera(pre)
		r.Render(pre, tex, cam, Decor(pre, th), uint64(d))
		r.DrawParticles(cam, uint64(d)*7)
		post := dungeon.Generate(seed, d)
		if !reflect.DeepEqual(plain[d], post) {
			t.Fatalf("depth %d: level differs after visual building", d)
		}
	}
	// Also every theme, base and remix, against the next depth's level.
	for i := range proc.Themes {
		for lap := range 3 {
			th := proc.Themes[i].Remix(lap)
			NewTextures(th, 9)
			r.SetWorld(th)
		}
	}
	for d := 1; d <= 25; d++ {
		if !reflect.DeepEqual(plain[d], dungeon.Generate(seed, d)) {
			t.Fatalf("depth %d: level differs after building every theme", d)
		}
	}
}

// Same seed gives the same frame, fresh renderer or reused.
func TestQARenderRepeatable(t *testing.T) {
	for _, depth := range []int{1, 4, 9, 13, 27} {
		l := dungeon.Generate(5, depth)
		th := proc.ThemeFor(depth)
		cam := startCamera(l)
		frame := func() []byte {
			r := New(96, 60)
			r.SetWorld(th)
			sp := Decor(l, th)
			r.Render(l, NewTextures(th, l.Seed), cam, sp, 40)
			r.DrawParticles(cam, 40)
			return slices.Clone(r.Img.Pix)
		}
		if !slices.Equal(frame(), frame()) {
			t.Errorf("depth %d: frames differ", depth)
		}
	}
}

// With Calm on, a frame is identical at every tick, in every world and lap.
func TestQACalmFrameIgnoresTick(t *testing.T) {
	r := New(96, 60)
	r.Calm = true
	for depth := 1; depth <= 30; depth++ {
		l := dungeon.Generate(11, depth)
		th := proc.ThemeFor(depth)
		tex := NewTextures(th, l.Seed)
		sp := Decor(l, th)
		cam := startCamera(l)
		r.SetWorld(th)
		var first []byte
		for _, tick := range []uint64{0, 7, 8, 59, 600, 12345, 1 << 40} {
			r.Render(l, tex, cam, sp, tick)
			r.DrawParticles(cam, tick)
			if first == nil {
				first = slices.Clone(r.Img.Pix)
			} else if !slices.Equal(first, r.Img.Pix) {
				t.Fatalf("depth %d (%s): calm frame changes at tick %d", depth, th.Name, tick)
			}
		}
	}
}

// Calm does not change the first frame's look versus tick 0 non-calm.
func TestQACalmMatchesTickZero(t *testing.T) {
	for depth := 1; depth <= 10; depth++ {
		l := dungeon.Generate(3, depth)
		th := proc.ThemeFor(depth)
		tex := NewTextures(th, l.Seed)
		cam := startCamera(l)
		r := New(96, 60)
		r.SetWorld(th)
		r.Render(l, tex, cam, nil, 0)
		want := slices.Clone(r.Img.Pix)
		r.Calm = true
		r.Render(l, tex, cam, nil, 999)
		if !slices.Equal(want, r.Img.Pix) {
			t.Errorf("%s: calm frame differs from the tick 0 frame", th.Name)
		}
	}
}

// Render and DrawParticles allocate nothing in every world on laps 1 to 3.
func TestQAZeroAllocsEveryLap(t *testing.T) {
	r := New(192, 120)
	for depth := 1; depth <= 30; depth++ {
		l := dungeon.Generate(21, depth)
		th := proc.ThemeFor(depth)
		tex := NewTextures(th, l.Seed)
		sp := Decor(l, th)
		cam := startCamera(l)
		r.SetWorld(th)
		tick := uint64(0)
		r.Render(l, tex, cam, sp, tick)
		r.DrawParticles(cam, tick)
		if n := testing.AllocsPerRun(10, func() {
			tick += 13
			r.Render(l, tex, cam, sp, tick)
			r.DrawParticles(cam, tick)
		}); n != 0 {
			t.Errorf("depth %d (%s): %v allocs a frame", depth, th.Name, n)
		}
	}
}

// Decor copes with degenerate levels: none, empty, no free floor.
func TestQADecorEdgeCases(t *testing.T) {
	for i := range proc.Themes {
		th := &proc.Themes[i]
		if got := Decor(&dungeon.Level{}, th); len(got) != 0 {
			t.Errorf("%s: empty level has %d props", th.Name, len(got))
		}
		// Rooms but a level whose tiles are all walls.
		l := dungeon.Generate(2, 1)
		for y := 0; y < l.H; y++ {
			for x := 0; x < l.W; x++ {
				l.Set(dungeon.Point{X: x, Y: y}, dungeon.Wall)
			}
		}
		if got := Decor(l, th); len(got) != 0 {
			t.Errorf("%s: solid level has %d props", th.Name, len(got))
		}
		// Rooms hanging off the map.
		l.Rooms = append(l.Rooms, dungeon.Room{X: -5, Y: -5, W: 3, H: 3}, dungeon.Room{X: l.W - 1, Y: l.H - 1, W: 6, H: 6}, dungeon.Room{W: 0, H: 0})
		Decor(l, th)
	}
	for _, depth := range []int{1, 2, 3, 12, 25} {
		l := dungeon.Generate(1, depth)
		Decor(l, proc.ThemeFor(depth))
	}
}

// Render copes with no free floor around the camera and a frame with a
// zero-sized or odd world: SetWorld on every theme/lap, huge depth theme.
func TestQAHugeDepthWorld(t *testing.T) {
	r := New(64, 40)
	for _, d := range []int{1000, 1 << 20, -4} {
		th := proc.ThemeFor(d)
		r.SetWorld(th)
		l := dungeon.Generate(1, 3)
		tex := NewTextures(th, 1)
		cam := startCamera(l)
		r.Render(l, tex, cam, Decor(l, th), 1<<40)
		r.DrawParticles(cam, 1<<40)
	}
}

// Particles never draw over a nearer sprite: with a sprite right in front
// of the camera, no pixel it covers changes when particles are drawn.
func TestQAParticlesHideBehindSprites(t *testing.T) {
	for i := range proc.Themes {
		for lap := range 2 {
			th := proc.World(i, lap)
			if th.Particles == proc.NoParticles {
				continue
			}
			l := dungeon.Generate(7, 1+slices.Index(proc.FloorOrder, i))
			cam := startCamera(l)
			r := New(192, 120)
			r.SetWorld(th)
			d := 0.51 // nearer than every particle but a sliver
			front := Sprite{X: cam.X + cam.DirX*d, Y: cam.Y + cam.DirY*d, Size: 1, Img: solidSprite()}
			covered := 0
			for tick := uint64(0); tick < 1200; tick += 37 {
				r.Render(l, NewTextures(th, l.Seed), cam, []Sprite{front}, tick)
				before := slices.Clone(r.Img.Pix)
				r.DrawParticles(cam, tick)
				for o := range r.spriteAt {
					if r.spriteAt[o] != r.frame {
						continue
					}
					covered++
					if !slices.Equal(before[o*4:o*4+4], r.Img.Pix[o*4:o*4+4]) {
						t.Fatalf("%s lap %d: a particle drew over a sprite at pixel %d", th.Name, lap, o)
					}
				}
			}
			if covered == 0 {
				t.Fatalf("%s: the test sprite covered nothing", th.Name)
			}
		}
	}
}

// Particles are still drawn where no sprite is (the clipping must not
// hide them all).
func TestQAParticlesStillShowWithSprites(t *testing.T) {
	for i := range proc.Themes {
		th := &proc.Themes[i]
		if th.Particles == proc.NoParticles {
			continue
		}
		l := dungeon.Generate(7, 1+slices.Index(proc.FloorOrder, i))
		cam := startCamera(l)
		r := New(192, 120)
		r.SetWorld(th)
		tex := NewTextures(th, l.Seed)
		shown := false
		for tick := uint64(0); tick < 600 && !shown; tick += 60 {
			r.Render(l, tex, cam, Decor(l, th), tick)
			before := slices.Clone(r.Img.Pix)
			r.DrawParticles(cam, tick)
			shown = !slices.Equal(before, r.Img.Pix)
		}
		if !shown {
			t.Errorf("%s: no particles with sprites present", th.Name)
		}
	}
}

// The luminance table matches the WCAG formula.
func TestQALuminanceTable(t *testing.T) {
	for v := range 256 {
		s := float64(v) / 255
		want := s / 12.92
		if s > 0.04045 {
			want = math.Pow((s+0.055)/1.055, 2.4)
		}
		if math.Abs(linear[v]-want) > 1e-12 {
			t.Fatalf("linear[%d] = %v, want %v", v, linear[v], want)
		}
		if v > 0 && linear[v] <= linear[v-1] {
			t.Fatalf("linear not increasing at %d", v)
		}
	}
}

// Each world's sprite light reaches the renderer, and is sane.
func TestQASpriteLight(t *testing.T) {
	r := New(32, 20)
	for i := range proc.Themes {
		for lap := range 3 {
			th := proc.World(i, lap)
			r.SetWorld(th)
			if r.spriteLight != th.SpriteMinLight() || r.spriteLight <= 0 || r.spriteLight >= 1 {
				t.Errorf("%s lap %d: sprite light %v", th.Name, lap, r.spriteLight)
			}
			if r.spriteLevel != int(r.spriteLight*(levels-1)) || r.spriteLevel < 0 || r.spriteLevel >= levels {
				t.Errorf("%s lap %d: sprite level %d", th.Name, lap, r.spriteLevel)
			}
		}
	}
}

// Director floors on later laps get the world's remix, and World agrees
// with ThemeFor.
func TestQAWorldLaps(t *testing.T) {
	for i := range proc.Themes {
		if proc.World(i, 0) != &proc.Themes[i] || proc.World(i, -3) != &proc.Themes[i] {
			t.Errorf("World(%d, 0) is not the base theme", i)
		}
		if proc.World(i, 1) == &proc.Themes[i] || proc.World(i, 2) == proc.World(i, 1) || proc.World(i, 3) != proc.World(i, 1) {
			t.Errorf("World(%d, lap) remix cycle wrong", i)
		}
		if proc.World(i, 1).Name != proc.Themes[i].Name {
			t.Errorf("remix renamed %s", proc.Themes[i].Name)
		}
	}
	for d := 1; d <= 60; d++ {
		if proc.ThemeFor(d) != proc.World(proc.FloorOrder[(d-1)%10], proc.Lap(d)) {
			t.Errorf("depth %d: ThemeFor and World disagree", d)
		}
	}
	if n := testing.AllocsPerRun(10, func() { proc.World(3, 2) }); n != 0 {
		t.Errorf("World allocates %v", n)
	}
}

// solidSprite is a fully opaque 8x8 sprite.
func solidSprite() *proc.Indexed {
	m := &proc.Indexed{W: 8, H: 8, Pix: make([]uint8, 64), Glow: make([]bool, 64)}
	for i := range m.Pix {
		m.Pix[i] = pal.Index(pal.White)
	}
	return m
}
