package raycast

import (
	"reflect"
	"slices"
	"testing"

	"github.com/halpworld/halpwords/internal/dungeon"
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
