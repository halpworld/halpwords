// Package game runs the main loop: it owns the scene stack, shared resources
// and pixel-perfect scaling to the window.
package game

import (
	"bytes"
	"fmt"
	"image/color"
	"math"
	"path"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/assets"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/save"
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
	Sound      *Sound

	scenes  *manager
	notice  string // a short message in the corner, like "Sound off"
	noticeT int
}

// Notify shows a short message in the top corner for a moment.
func (c *Context) Notify(msg string) { c.notice, c.noticeT = msg, 90 }

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

// WordsDir is the folder, inside the user's folder, that holds their own
// word lists.
const WordsDir = "words"

// StarterLists returns the word lists built into the game.
func StarterLists() ([]*words.List, error) {
	lists, err := words.LoadFS(assets.Words, "words")
	if err != nil {
		return nil, fmt.Errorf("starter word lists: %w", err)
	}
	return lists, nil
}

// UserLists returns the user's own word lists, and describes the files that
// could not be read.
func UserLists() (lists []*words.List, errs []string) {
	names, err := save.List(WordsDir)
	if err != nil {
		if _, dirErr := save.Dir(); dirErr != nil {
			return nil, nil // a web browser with no stored lists
		}
		return nil, []string{err.Error()}
	}
	for _, n := range names {
		if !strings.EqualFold(path.Ext(n), ".txt") {
			continue
		}
		data, err := save.Read(WordsDir + "/" + n)
		if err == nil {
			var l *words.List
			if l, err = words.Parse(bytes.NewReader(data), n); err == nil {
				lists = append(lists, l)
				continue
			}
		}
		errs = append(errs, err.Error())
	}
	return lists, errs
}

// LoadLists (re)loads the starter word lists and the user's own. A user list
// with the same file name as a starter list replaces it.
func (c *Context) LoadLists() error {
	starters, err := StarterLists()
	if err != nil {
		return err
	}
	user, errs := UserLists()
	own := map[string]bool{}
	for _, l := range user {
		own[l.Source] = true
	}
	c.Lists = nil
	for _, l := range starters {
		if !own[l.Source] {
			c.Lists = append(c.Lists, l)
		}
	}
	c.Lists = append(c.Lists, user...)
	c.ListErrors = errs
	return nil
}

// UserDir is where saves, settings and the user's own word lists live, e.g.
// ~/Library/Application Support/halpwords on macOS.
func UserDir() (string, error) { return save.Dir() }

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
		Sound:  newSound(),
		scenes: &manager{},
	}
	if err := ctx.LoadLists(); err != nil {
		return nil, err
	}
	ctx.scenes.stack = []Scene{first(ctx)}
	return &Game{ctx: ctx}, nil
}

// Update implements ebiten.Game.
func (g *Game) Update() error {
	g.ctx.Tick++
	g.ctx.Input.Update()
	g.ctx.Sound.update(g.ctx.Tick)
	if g.ctx.noticeT > 0 {
		g.ctx.noticeT--
	}
	if input.Pressed(ebiten.KeyF3) {
		switch {
		case !g.ctx.Sound.Available():
			g.ctx.Notify("No sound device")
		case g.ctx.Sound.Toggle():
			g.ctx.Notify("Sound on")
		default:
			g.ctx.Notify("Sound off")
		}
		return g.ctx.scenes.update(g.ctx)
	}
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
	if c := g.ctx; c.noticeT > 0 {
		a := min(1, float64(c.noticeT)/20)
		w := c.Font.Width(c.notice, 1) + 16
		gfx.FillRect(screen, ScreenW-w-4, 4, w, 22, pal.Fade(pal.Black, 0.75*a))
		c.Font.DrawShadow(screen, c.notice, ScreenW-w+4, 7, 1, pal.Fade(pal.Yellow, a))
	}
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
