// Package game runs the main loop: it owns the scene stack, shared resources
// and pixel-perfect scaling to the window.
package game

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"math"
	"path"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/assets"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/llm"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/profile"
	"github.com/halpworld/halpwords/internal/report"
	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/internal/unifont"
	"github.com/halpworld/halpwords/pkg/words"
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
	// Profile is what lasts between adventures: settings, what the player
	// knows of each word, and the Hall of Fame.
	Profile *profile.Profile
	// AI is the connection to a language model, when one is set up. The
	// game plays the same without it.
	AI *llm.Service
	// Link is the link to a grown-up's account on the website. It is
	// never nil, and does nothing until the game is linked.
	Link *link.Client
	// Reports queues the pause menu's reports and sends them to
	// Halpwords when the game is online. It may be nil (in tests).
	Reports *report.Outbox

	scenes  *manager
	notice  string // a short message in the corner, like "Sound off"
	noticeT int
	opts    profile.Options // the game settings in use
	// linkSeen is the link's change count last picked up; session is the
	// play session going on.
	linkSeen int
	session  *session

	// Full screen changes wait for the one before to finish: on macOS,
	// changing again during the animation crashes the app.
	fullWant    bool // whether full screen is wanted
	fullPending bool // fullWant is not yet put into effect
	fullWait    int  // ticks until full screen may change again
}

// ApplyOptions puts the game settings in the profile into effect: the
// volumes, the CRT filter and full screen.
func (c *Context) ApplyOptions() {
	c.opts = c.Profile.Settings.Options()
	c.Sound.Music = float64(c.opts.Music) / profile.MaxVolume
	c.Sound.Effects = float64(c.opts.Effects) / profile.MaxVolume
	c.Sound.music.volume(c.Sound)
	c.fullWant, c.fullPending = c.opts.Fullscreen, true
	c.syncFullscreen()
}

// syncFullscreen switches full screen to what is wanted, once the last
// switch has had time to finish. Quick changes in a row become one.
func (c *Context) syncFullscreen() {
	if c.fullWait > 0 {
		if c.fullWait--; c.fullWait == 0 {
			input.ForgetHeld() // keys released during the switch may be lost
		}
		return
	}
	if !c.fullPending {
		return
	}
	c.fullPending = false
	if ebiten.IsFullscreen() != c.fullWant {
		input.ForgetHeld()
		ebiten.SetFullscreen(c.fullWant)
		c.fullWait = ebiten.TPS() // the macOS animation takes about half a second
	}
}

// isFullscreen reports whether full screen is on, or soon will be.
func (c *Context) isFullscreen() bool {
	if c.fullPending || c.fullWait > 0 {
		return c.fullWant
	}
	return ebiten.IsFullscreen()
}

// Shake reports whether the view may shake. Some players turn it off.
func (c *Context) Shake() bool { return c.opts.Shake }

// toggleFullscreen switches full screen on or off and remembers it.
func (c *Context) toggleFullscreen() {
	o := c.Profile.Settings.Options()
	o.Fullscreen = !c.isFullscreen()
	c.Profile.Settings.SetOptions(o)
	c.Profile.SaveSettings()
	c.ApplyOptions()
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
		own[l.File] = true
	}
	c.Lists = nil
	for _, l := range starters {
		if !own[l.File] {
			c.Lists = append(c.Lists, l)
		}
	}
	c.Lists = append(c.Lists, user...)
	c.Lists = append(c.Lists, c.Link.Lists()...) // assigned lists
	c.ListErrors = errs
	return nil
}

// saveStore keeps the AI settings in the user's folder.
type saveStore struct{}

func (saveStore) Read(name string) ([]byte, error)            { return save.Read(name) }
func (saveStore) Write(name string, data []byte) error        { return save.Write(name, data) }
func (saveStore) WritePrivate(name string, data []byte) error { return save.WritePrivate(name, data) }

// NewOutbox returns the report queue in the user's folder, sending to
// report.ServerURL(). Reports are anonymous: the game isn't linked to an
// account yet. The link client (W1.8) sets Sender.Token to its access
// token, "" when unlinked.
func NewOutbox() *report.Outbox {
	return &report.Outbox{
		Queue:  report.NewQueue(saveStore{}),
		Sender: &report.Sender{Server: report.ServerURL(), UserAgent: UserAgent()},
	}
}

// UserDir is where saves, settings and the user's own word lists live, e.g.
// ~/Library/Application Support/halpwords on macOS.
func UserDir() (string, error) { return save.Dir() }

// Game implements ebiten.Game.
type Game struct {
	ctx *Context
	crt crt
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
	ctx.openLink()
	if err := ctx.LoadLists(); err != nil {
		return nil, err
	}
	prof, errs := profile.Load()
	ctx.Profile = prof
	if len(errs) > 0 {
		ctx.Notify("Some saved progress was damaged")
	}
	ctx.lockSettings()
	ctx.linkSeen = ctx.Link.Changes()
	ctx.ApplyOptions()
	ctx.AI = llm.Load(saveStore{})
	if p := ctx.AI.Provider(); p != nil {
		ctx.AI.Check(p.ID) // free: it lists the models the key can use
	}
	ctx.Reports = NewOutbox()
	go ctx.Reports.Flush(context.Background()) // reports left from last time
	ctx.scenes.stack = []Scene{first(ctx)}
	return &Game{ctx: ctx}, nil
}

// Update implements ebiten.Game.
func (g *Game) Update() error {
	defer guard()
	g.ctx.Tick++
	g.ctx.syncFullscreen()
	g.ctx.Input.Update()
	g.ctx.Sound.update(g.ctx.Tick)
	g.ctx.pollLink()
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
		(input.Pressed(ebiten.KeyEnter) && input.Held(ebiten.KeyAlt)) ||
		(input.Pressed(ebiten.KeyF) && input.Held(ebiten.KeyMeta) && input.Held(ebiten.KeyControl)) {
		g.ctx.toggleFullscreen()
		g.ctx.Input.Chars = g.ctx.Input.Chars[:0]
		return nil
	}
	err := g.ctx.scenes.update(g.ctx)
	g.ctx.Sound.PlayMusic(g.ctx.scenes.music())
	return err
}

// Draw implements ebiten.Game.
func (g *Game) Draw(screen *ebiten.Image) {
	defer guard()
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
	x, y := math.Floor((sw-ScreenW*scale)/2), math.Floor((sh-ScreenH*scale)/2)
	if c := g.ctx.opts.CRT; c != profile.CRTOff && g.crt.draw(screen, offscreen, x, y, ScreenW*scale, ScreenH*scale, scale, c) {
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(x, y)
	op.Filter = ebiten.FilterNearest
	screen.DrawImage(offscreen, op)
}
