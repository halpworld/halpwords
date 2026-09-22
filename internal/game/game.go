// Package game runs the main loop: it owns the scene stack, shared resources
// and pixel-perfect scaling to the window.
package game

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/assets"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/unifont"
	"github.com/halpworld/halpwords/internal/words"
)

// The logical screen size. Everything is drawn at this size and then scaled.
const (
	ScreenW = 640
	ScreenH = 360
)

// Scene is one screen of the game: title, practice, dungeon, battle...
type Scene interface {
	Update(ctx *Context) error
	Draw(dst *ebiten.Image, ctx *Context)
}

// Context is shared by all scenes.
type Context struct {
	Font  *gfx.Font
	Input *input.State
	Lists []*words.List
	// ListErrors describes word list files that failed to load, so teachers
	// can see what to fix.
	ListErrors []string
	Tick       uint64

	scenes *manager
}

// Push shows s on top of the current scene.
func (c *Context) Push(s Scene) {
	c.scenes.transition(func() { c.scenes.stack = append(c.scenes.stack, s) })
}

// Pop returns to the previous scene.
func (c *Context) Pop() {
	c.scenes.transition(func() { c.scenes.stack = c.scenes.stack[:len(c.scenes.stack)-1] })
}

// Replace swaps the current scene for s.
func (c *Context) Replace(s Scene) {
	c.scenes.transition(func() { c.scenes.stack[len(c.scenes.stack)-1] = s })
}

// ListsFor returns the word lists for a language code.
func (c *Context) ListsFor(code string) []*words.List {
	var out []*words.List
	for _, l := range c.Lists {
		if l.Language == code {
			out = append(out, l)
		}
	}
	return out
}

// UserDir is where saves, settings and the user's own word lists live, e.g.
// ~/Library/Application Support/halpwords on macOS.
func UserDir() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "halpwords"), nil
}

// Game implements ebiten.Game.
type Game struct {
	ctx *Context
}

// New loads shared resources and starts with the scene made by first.
func New(first func(*Context) Scene) (*Game, error) {
	face, err := unifont.ParseBytes(assets.UnifontHex)
	if err != nil {
		return nil, err
	}
	ctx := &Context{
		Font:   gfx.NewFont(face),
		Input:  &input.State{},
		scenes: &manager{},
	}
	ctx.Lists, err = words.LoadFS(assets.Words, "words")
	if err != nil {
		return nil, fmt.Errorf("starter word lists: %w", err)
	}
	if dir, err := UserDir(); err == nil {
		wdir := filepath.Join(dir, "words")
		if _, err := os.Stat(wdir); err == nil {
			user, err := words.LoadFS(os.DirFS(wdir), ".")
			ctx.Lists = append(ctx.Lists, user...)
			if err != nil {
				ctx.ListErrors = append(ctx.ListErrors, err.Error())
			}
		}
	}
	ctx.scenes.stack = []Scene{first(ctx)}
	return &Game{ctx: ctx}, nil
}

// Update implements ebiten.Game.
func (g *Game) Update() error {
	g.ctx.Tick++
	g.ctx.Input.Update()
	if input.Pressed(ebiten.KeyF11) ||
		(input.Pressed(ebiten.KeyEnter) && ebiten.IsKeyPressed(ebiten.KeyAlt)) ||
		(input.Pressed(ebiten.KeyF) && ebiten.IsKeyPressed(ebiten.KeyMeta) && ebiten.IsKeyPressed(ebiten.KeyControl)) {
		ebiten.SetFullscreen(!ebiten.IsFullscreen())
		g.ctx.Input.Chars = g.ctx.Input.Chars[:0]
		return nil
	}
	return g.ctx.scenes.update(g.ctx)
}

// Draw implements ebiten.Game.
func (g *Game) Draw(screen *ebiten.Image) {
	g.ctx.scenes.draw(screen, g.ctx)
}

// Layout implements ebiten.Game. The logical screen never changes size.
func (g *Game) Layout(int, int) (int, int) { return ScreenW, ScreenH }

// DrawFinalScreen scales the logical screen by the largest whole number that
// fits the window, so pixels stay square and sharp, and letterboxes the rest.
func (g *Game) DrawFinalScreen(screen ebiten.FinalScreen, offscreen *ebiten.Image, _ ebiten.GeoM) {
	screen.Fill(color.Black)
	sw, sh := float64(screen.Bounds().Dx()), float64(screen.Bounds().Dy())
	scale := math.Min(sw/ScreenW, sh/ScreenH)
	if scale >= 1 {
		scale = math.Floor(scale)
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(math.Floor((sw-ScreenW*scale)/2), math.Floor((sh-ScreenH*scale)/2))
	op.Filter = ebiten.FilterNearest
	screen.DrawImage(offscreen, op)
}
