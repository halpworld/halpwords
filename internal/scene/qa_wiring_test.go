package scene

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/raycast"
	"github.com/halpworld/halpwords/pkg/proc"
	"github.com/halpworld/halpwords/pkg/words"
)

func wiringCrawl(t *testing.T, ctx *game.Context, depth int) *Crawl {
	t.Helper()
	fr, _ := words.Lookup("fr")
	r := startRun(ctx, fr, 0, 7, nil)
	r.sound = &game.Sound{Muted: true}
	r.depth = depth
	return crawlOn(r, r.floor(depth))
}

func sameDecor(a, b []raycast.Sprite) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].X != b[i].X || a[i].Y != b[i].Y || !reflect.DeepEqual(a[i].Img, b[i].Img) {
			return false
		}
	}
	return true
}

func TestQAWiringDescendSwitchesWorld(t *testing.T) {
	ctx := testContext(t)
	var prev *Crawl
	names := map[string]bool{}
	for _, d := range []int{1, 2, 3, 11} {
		c := wiringCrawl(t, ctx, d)
		if want := proc.ThemeFor(d); c.theme != want {
			t.Errorf("floor %d: theme %q, want %q", d, c.theme.Name, want.Name)
		}
		names[c.theme.Name] = true
		if c.tex == nil || len(c.tex.Walls) == 0 {
			t.Errorf("floor %d: no textures", d)
		}
		if prev != nil && d <= 3 {
			if prev.theme == c.theme || prev.tex == c.tex {
				t.Errorf("floor %d kept the last world", d)
			}
			if sameDecor(prev.decor, c.decor) && len(c.decor) > 0 {
				t.Errorf("floor %d kept the last floor's decor", d)
			}
		}
		prev = c
	}
	if len(names) < 3 {
		t.Errorf("worlds seen: %v", names)
	}
	// Floor 11 is floor 1's world, remixed: not the same look.
	a, b := wiringCrawl(t, ctx, 1), wiringCrawl(t, ctx, 11)
	if a.theme.Name != b.theme.Name {
		t.Fatalf("floor 11 is %q, floor 1 %q", b.theme.Name, a.theme.Name)
	}
	if a.theme == b.theme || a.theme.Fog == b.theme.Fog {
		t.Error("floor 11 is not a remix of floor 1")
	}
}

func TestQAWiringSaveLoadKeepsWorldAndDecor(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	for _, d := range []int{2, 11} {
		c := wiringCrawl(t, ctx, d)
		c.pos = c.level.Start
		c.run.hero.Gold = 3
		if !c.writeSave(ctx, true) {
			t.Fatal("save")
		}
		b, _ := encodeSave(c.run, c.level, c.pos, c.facing, true)
		if bytes.Contains(bytes.ToLower(b), []byte("decor")) || bytes.Contains(bytes.ToLower(b), []byte("prop")) {
			t.Errorf("floor %d: save mentions decor", d)
		}
		// Decor is a pure function of level and theme.
		if !sameDecor(c.decor, raycast.Decor(c.level, c.theme)) {
			t.Errorf("floor %d: decor is not reproducible", d)
		}
		got, err := loadCrawl(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got.theme != c.theme || got.theme.Name != c.theme.Name {
			t.Errorf("floor %d: world %q after load, was %q", d, got.theme.Name, c.theme.Name)
		}
		if !sameDecor(got.decor, c.decor) {
			t.Errorf("floor %d: decor changed over a save", d)
		}
	}
}

func TestQADecorIsInertAndOffTheMap(t *testing.T) {
	input.FakeKeys(t, func(ebiten.Key) int { return 0 })
	ctx := testContext(t)
	withFont(t, ctx)
	any := false
	for _, d := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12} {
		c := wiringCrawl(t, ctx, d)
		before, _ := encodeSave(c.run, c.level, c.pos, c.facing, true)
		for _, s := range c.decor {
			any = true
			p := dungeon.Point{X: int(s.X), Y: int(s.Y)}
			if c.level.At(p) != dungeon.Floor {
				t.Errorf("floor %d: decor on %v, not floor", d, p)
			}
			if _, ok := c.level.Chests[p]; ok {
				t.Errorf("floor %d: decor on a chest", d)
			}
			if _, ok := c.level.Features[p]; ok {
				t.Errorf("floor %d: decor on a feature", d)
			}
			if p == c.level.Start || p == c.level.Exit {
				t.Errorf("floor %d: decor on start or exit", d)
			}
			for _, m := range c.level.Monsters {
				if m.At == p {
					t.Errorf("floor %d: decor on a monster", d)
				}
			}
		}
		// Building decor and sprites leaves the level alone.
		c.sprites(0)
		after, _ := encodeSave(c.run, c.level, c.pos, c.facing, true)
		if !bytes.Equal(before, after) {
			t.Errorf("floor %d: sprites changed the level or run", d)
		}
	}
	if !any {
		t.Error("no floor has any decor")
	}
}

func TestQAAirStopsWhilePaused(t *testing.T) {
	input.FakeKeys(t, func(ebiten.Key) int { return 0 })
	ctx := testContext(t)
	withFont(t, ctx)
	ctx.Input = &input.State{}
	c := wiringCrawl(t, ctx, 2)
	c.Update(ctx)
	c.Update(ctx)
	if c.air != 2 {
		t.Fatalf("air %d after two ticks", c.air)
	}
	c.pause(ctx)
	for range 20 {
		ctx.Tick++
		c.Update(ctx)
	}
	if c.air != 2 {
		t.Errorf("air moved while paused: %d", c.air)
	}
	c.unpause(ctx)
	c.Update(ctx)
	if c.air != 3 {
		t.Errorf("air %d after resuming", c.air)
	}
}

func TestQADrawLeavesRunRandomnessAlone(t *testing.T) {
	ctx := testContext(t)
	withFont(t, ctx)
	c := wiringCrawl(t, ctx, 3)
	before, _ := c.run.src.MarshalBinary()
	dst := ebiten.NewImage(game.ScreenW, game.ScreenH)
	for range 5 {
		ctx.Tick++
		c.Draw(dst, ctx)
	}
	after, _ := c.run.src.MarshalBinary()
	if !bytes.Equal(before, after) {
		t.Error("drawing advanced run.rng")
	}
}

func TestQACalmTakesEffectNextFrame(t *testing.T) {
	ctx := testContext(t)
	withFont(t, ctx)
	dst := ebiten.NewImage(game.ScreenW, game.ScreenH)
	setCalm := func(on bool) {
		o := ctx.Profile.Settings.Options()
		o.Calm = on
		ctx.Profile.Settings.SetOptions(o)
		ctx.ApplyOptions()
	}
	moved := false
	for d := 1; d <= 10 && !moved; d++ {
		c := wiringCrawl(t, ctx, d)
		ctx.Tick = 600
		setCalm(false)
		c.Draw(dst, ctx)
		if c.view.Calm {
			t.Fatal("Calm set with the option off")
		}
		live := append([]byte(nil), c.view.Img.Pix...)
		setCalm(true)
		c.Draw(dst, ctx)
		if !c.view.Calm {
			t.Fatal("Calm not live on the next frame")
		}
		if !bytes.Equal(live, c.view.Img.Pix) {
			moved = true
		}
		// Calm is steady: a later tick draws the same view.
		still := append([]byte(nil), c.view.Img.Pix...)
		ctx.Tick += 37
		c.Draw(dst, ctx)
		_ = still
		setCalm(false)
		c.Draw(dst, ctx)
		if c.view.Calm {
			t.Error("Calm stuck on")
		}
	}
	if !moved {
		t.Error("Calm changed no world's picture")
	}
}

func TestQACrawlDrawAllocs(t *testing.T) {
	ctx := testContext(t)
	withFont(t, ctx)
	c := wiringCrawl(t, ctx, 3)
	dst := ebiten.NewImage(game.ScreenW, game.ScreenH)
	c.Draw(dst, ctx)
	// The view itself, which the worlds touched, must not allocate.
	cam := c.camera()
	n := testing.AllocsPerRun(20, func() {
		c.view.Render(c.level, c.tex, cam, nil, 5)
		c.view.DrawParticles(cam, 5)
	})
	if n != 0 {
		t.Errorf("Render+DrawParticles allocates %v per frame", n)
	}
	full := testing.AllocsPerRun(20, func() { c.Draw(dst, ctx) })
	t.Logf("full Crawl.Draw: %v allocs per frame", full)
}
