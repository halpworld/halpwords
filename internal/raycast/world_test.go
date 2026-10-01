package raycast

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"os"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/pkg/proc"
)

// worldScene is one floor of a world, ready to draw from its start.
type worldScene struct {
	theme   *proc.Theme
	level   *dungeon.Level
	tex     *Textures
	cam     Camera
	sprites []Sprite
}

// sceneFor builds the first floor that has world i, with its monsters and
// props.
func sceneFor(i int) worldScene {
	depth := slices.Index(proc.FloorOrder, i) + 1
	l := dungeon.Generate(7, depth)
	th := proc.ThemeFor(depth)
	s := worldScene{theme: th, level: l, tex: NewTextures(th, l.Seed), cam: startCamera(l)}
	for _, m := range l.Monsters {
		s.sprites = append(s.sprites, Sprite{X: float64(m.At.X) + 0.5, Y: float64(m.At.Y) + 0.5, Size: m.Kind.Size,
			Img: proc.MonsterSprite(m.Kind.Family, m.Kind.Hue, m.Seed, 0)})
	}
	s.sprites = append(s.sprites, Decor(l, th)...)
	return s
}

// draw renders the scene and its particles at tick.
func (s worldScene) draw(r *Renderer, sprites []Sprite, tick uint64) {
	r.Render(s.level, s.tex, s.cam, sprites, tick)
	r.DrawParticles(s.cam, tick)
}

// Each world's 192×120 view from a fixed camera, monsters, props and
// particles included, so a change to how a world renders is always on
// purpose. Update a hash only when the new look is meant. Platforms that
// fuse multiply-adds (arm64) differ from the rest (amd64, wasm) by a few
// pixels, so each kind has its own hashes.
func TestWorldViewsPinned(t *testing.T) {
	want := map[string]string{
		"The Crypt":          "f398f0a2d834f38f12e6f7e329f8881059b3fc79bf16e7d89247eaac96381c16",
		"Mossy Cellars":      "93d4b579d98063ccad17d5768bd0322f40fd32467d34efa4d4a0bd10cf15d303",
		"Flooded Caves":      "73403b0f71df63921ac7d708150d9152f5ffab7992cca0e4c044c14c7e84466c",
		"Ice Halls":          "3fcadb3d399e5344554e1406ab85b25d199a8aa904ae8b456279d1944b12975d",
		"Lava Forge":         "76d6836391d70e2352e0f8b3aed7ebc46621af8394e3248554dc0fa666b55216",
		"Amethyst Vaults":    "444974aedb48ba6957ed6a3d8433ec66cb7ef635022c26babff13a48f445c2aa",
		"Clockwork Workshop": "b2fb2fa9c38f8e5b09e0ebe0cc30069eb5b8469b209e2317a3e01f0bdfc4aaa3",
		"Sky Garden":         "de5e690d2e81a374c515f808f85a980de858969f395043ea9fb1c5ef892df036",
		"Sandstone Tomb":     "a3ddaae900e8270884f12bb4d441449cc238f0536aaa9d7ea81f78d73a492f6a",
		"Whispering Library": "99e87f859289de08f6383156a705531f04d3d83610d6f07d892bc746bd5404c3",
	}
	if fusedMultiplyAdd {
		want = fusedWant
	}
	for i := range proc.Themes {
		s := sceneFor(i)
		r := New(192, 120)
		r.SetWorld(s.theme)
		s.draw(r, s.sprites, 600)
		sum := sha256.Sum256(r.Img.Pix)
		if got := hex.EncodeToString(sum[:]); got != want[s.theme.Name] {
			t.Errorf("%q: %q,", s.theme.Name, got)
		}
	}
}

// fusedWant are TestWorldViewsPinned's hashes where multiply-adds fuse.
var fusedWant = map[string]string{
	"The Crypt":          "f398f0a2d834f38f12e6f7e329f8881059b3fc79bf16e7d89247eaac96381c16",
	"Mossy Cellars":      "85379a77247ba1b1f19c78893959c9c13e2522ffe2ea791262ca3eba05b9bf0c",
	"Flooded Caves":      "73403b0f71df63921ac7d708150d9152f5ffab7992cca0e4c044c14c7e84466c",
	"Ice Halls":          "3fcadb3d399e5344554e1406ab85b25d199a8aa904ae8b456279d1944b12975d",
	"Lava Forge":         "76d6836391d70e2352e0f8b3aed7ebc46621af8394e3248554dc0fa666b55216",
	"Amethyst Vaults":    "877cfacb7aebdb92d1bd8df65312f7248d4ecb07f1ad965f206c4a39632effbf",
	"Clockwork Workshop": "b2fb2fa9c38f8e5b09e0ebe0cc30069eb5b8469b209e2317a3e01f0bdfc4aaa3",
	"Sky Garden":         "de5e690d2e81a374c515f808f85a980de858969f395043ea9fb1c5ef892df036",
	"Sandstone Tomb":     "a3ddaae900e8270884f12bb4d441449cc238f0536aaa9d7ea81f78d73a492f6a",
	"Whispering Library": "2fc86663f754fbf0a22edd13ed0aa658b271bb77f5310e398c5b62d671ab2c1d",
}

// A monster's body stays readable in every world, at the least light a
// sprite gets: at least 1.5:1 from the fog, and at middle distance from the
// world's main wall and floor colours. On a light surface (luminance 0.1
// or more) the black outline is enough; on a dark one the body itself must
// stand out. Every lap's remix is checked too.
func TestMonstersStandOut(t *testing.T) {
	const want = 1.5
	r := New(192, 120)
	against := func(body, surface float64) float64 {
		c := contrast(body, surface)
		if surface >= outlineLight {
			c = max(c, contrast(luminance(pal.Black), surface))
		}
		return c
	}
	for i := range proc.Themes {
		for lap := range 3 {
			th := proc.Themes[i].Remix(lap)
			r.SetWorld(th)
			fog := luminance(th.Fog)
			// Middle distance is half the light's reach.
			mid := lightLevel(0.5)
			wall := luminance(pal.All[r.shade[mid][mainColour(th.WallTex(0, 0))]])
			floor := luminance(pal.All[r.shade[mid][mainColour(th.FloorTex(0))]])
			for hue := range proc.MonsterHues() {
				body := pal.Index(proc.MonsterBody(hue))
				far := luminance(pal.All[r.spriteShade[r.spriteLevel][body]])
				if c := contrast(far, fog); c < want {
					t.Errorf("%s lap %d hue %d: body against the fog %.2f, want %.1f", th.Name, lap, hue, c, want)
				}
				near := luminance(pal.All[r.spriteShade[mid][body]])
				if c := against(near, wall); c < want {
					t.Errorf("%s lap %d hue %d: body against the wall %.2f, want %.1f", th.Name, lap, hue, c, want)
				}
				if c := against(near, floor); c < want {
					t.Errorf("%s lap %d hue %d: body against the floor %.2f, want %.1f", th.Name, lap, hue, c, want)
				}
			}
		}
	}
}

// lightLevel is the light level at brightness b, as dithering rounds it
// down.
func lightLevel(b float64) int { return int(b * (levels - 1)) }

// Drawing a frame allocates nothing, in any world, with or without
// sprites.
func TestRenderDoesNotAllocate(t *testing.T) {
	r := New(192, 120)
	for i := range proc.Themes {
		s := sceneFor(i)
		r.SetWorld(s.theme)
		tick := uint64(0)
		for _, sprites := range [][]Sprite{nil, s.sprites} {
			s.draw(r, sprites, tick) // warm up the reused buffers
			if n := testing.AllocsPerRun(20, func() {
				tick++
				s.draw(r, sprites, tick)
			}); n != 0 {
				t.Errorf("%s (%d sprites): %v allocations a frame", s.theme.Name, len(sprites), n)
			}
		}
	}
}

// No world costs more than twice The Crypt's frame time. Shared CI
// machines are too noisy for timings, so it only runs locally.
func TestWorldsCostTheSame(t *testing.T) {
	if testing.Short() || raceEnabled || os.Getenv("CI") != "" {
		t.Skip("timing test")
	}
	median := func(i int) time.Duration {
		s := sceneFor(i)
		r := New(192, 120)
		r.SetWorld(s.theme)
		times := make([]time.Duration, 41)
		for k := range times {
			start := time.Now()
			for range 10 {
				s.draw(r, nil, uint64(k))
			}
			times[k] = time.Since(start)
		}
		slices.Sort(times)
		return times[len(times)/2]
	}
	crypt := median(0)
	for i := 1; i < len(proc.Themes); i++ {
		if d := median(i); d > 2*crypt {
			t.Errorf("%s: %v for 10 frames, The Crypt %v", proc.Themes[i].Name, d, crypt)
		}
	}
}

// Props only read the level: placing them changes nothing about the floor.
func TestDecorLeavesTheLevelAlone(t *testing.T) {
	for i := range proc.Themes {
		depth := slices.Index(proc.FloorOrder, i) + 1
		for seed := range uint64(5) {
			l := dungeon.Generate(seed, depth)
			before := dungeon.Generate(seed, depth)
			a := Decor(l, proc.ThemeFor(depth))
			b := Decor(l, proc.ThemeFor(depth))
			if !reflect.DeepEqual(l, before) {
				t.Fatalf("%s seed %d: Decor changed the level", proc.Themes[i].Name, seed)
			}
			if !reflect.DeepEqual(a, b) {
				t.Fatalf("%s seed %d: Decor is not repeatable", proc.Themes[i].Name, seed)
			}
			for _, s := range a {
				p := dungeon.Point{X: int(s.X), Y: int(s.Y)}
				if l.At(p) != dungeon.Floor || p == l.Start || p == l.Exit || l.Chests[p] != nil || l.Features[p] != nil {
					t.Errorf("%s seed %d: a prop stands on %v", proc.Themes[i].Name, seed, p)
				}
			}
		}
	}
}

// Calm turns the particles off; otherwise every world with particles
// shows some.
func TestCalmStopsParticles(t *testing.T) {
	r := New(192, 120)
	for i := range proc.Themes {
		s := sceneFor(i)
		r.SetWorld(s.theme)
		changed := func(calm bool) bool {
			r.Calm = calm
			r.Render(s.level, s.tex, s.cam, nil, 0)
			before := slices.Clone(r.Img.Pix)
			shown := false
			for tick := uint64(0); tick < 600 && !shown; tick += 60 {
				r.DrawParticles(s.cam, tick)
				shown = !slices.Equal(before, r.Img.Pix)
			}
			return shown
		}
		if changed(true) {
			t.Errorf("%s: particles drawn while calm", s.theme.Name)
		}
		if want := s.theme.Particles != proc.NoParticles; changed(false) != want {
			t.Errorf("%s: particles shown %v, want %v", s.theme.Name, !want, want)
		}
	}
	r.Calm = false
}

// Snow and petals drift down gently: under 20 pixels a second at the
// nearest they are drawn, in a 192-pixel view.
func TestSlowFall(t *testing.T) {
	k := 192 / (2 * 0.75) // pixels per unit of height at distance 1
	for _, p := range []proc.Particles{proc.ParticleSnow, proc.ParticlePetals} {
		st := particleStyles[p]
		speed := (-st.fall + st.flutter*0.1*1.3) * k / particleNear
		if speed >= 20 {
			t.Errorf("particles %d fall %.1f pixels a second", p, speed)
		}
	}
}

// BenchmarkRenderWorlds draws a frame of each world with the floor's
// monsters and props, and its particles.
func BenchmarkRenderWorlds(b *testing.B) {
	for i := range proc.Themes {
		s := sceneFor(i)
		b.Run(s.theme.Name, func(b *testing.B) {
			r := New(192, 120)
			r.SetWorld(s.theme)
			b.ReportAllocs()
			b.ResetTimer()
			for k := range b.N {
				s.draw(r, s.sprites, uint64(k))
			}
		})
	}
}

// The written-out linearisation table matches the sRGB formula.
func TestLinearTable(t *testing.T) {
	for v := range 256 {
		s := float64(v) / 255
		want := s / 12.92
		if s > 0.04045 {
			want = math.Pow((s+0.055)/1.055, 2.4)
		}
		if math.Abs(linear[v]-want) > 1e-12 {
			t.Errorf("linear[%d] = %v, want %v", v, linear[v], want)
		}
	}
}

// Particles never draw over a sprite that stands in front of them.
func TestParticlesHideBehindSprites(t *testing.T) {
	s := sceneFor(4) // the Lava Forge, full of embers
	r := New(192, 120)
	r.SetWorld(s.theme)
	// A wall of sprite right in front of the camera covers the view.
	img := proc.NewIndexed(8, 8)
	img.Rect(0, 0, 8, 8, pal.White)
	near := []Sprite{{X: s.cam.X + s.cam.DirX*0.3, Y: s.cam.Y + s.cam.DirY*0.3, Img: img, Size: 3, Lift: -1}}
	for tick := uint64(0); tick < 600; tick += 30 {
		r.Render(s.level, s.tex, s.cam, near, tick)
		before := slices.Clone(r.Img.Pix)
		r.DrawParticles(s.cam, tick)
		for o := 0; o < len(before); o += 4 {
			if r.spriteAt[o/4] == r.frame && !slices.Equal(before[o:o+4], r.Img.Pix[o:o+4]) {
				t.Fatalf("tick %d: a particle drew over the sprite at pixel %d", tick, o/4)
			}
		}
	}
}
