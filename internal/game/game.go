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
	"sync"
	"sync/atomic"

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
	// Learners are the learners who play on this computer, each with
	// their own folder (W2.5). It is nil when the list couldn't be
	// read: the game then plays with the user's folder, as one learner.
	Learners *profile.Learners

	scenes  *manager
	notice  string // a short message in the corner, like "Sound off"
	noticeT int
	opts    profile.Options // the game settings in use
	// linkSeen is the link's change count last picked up; session is the
	// play session going on.
	linkSeen int
	session  *session
	// sendAs is Link, copied every frame, for the report outbox, which
	// sends in the background (reportToken); nil until
	// SendReportsAsLink.
	sendAs *atomic.Pointer[link.Client]
	// closing are links of learners switched away from, still closing
	// in the background, by folder.
	closing map[save.Folder]chan struct{}
	// addedFrom is the learner who played before AddLearner added
	// the one playing now, for CancelAddLearner.
	addedFrom string
	addedID   string
	// adopting is the learner whose folder the learner playing now is
	// signing in to open again, until the server says who signed in.
	adopting string
	// needWho is set when the game started on a guest, for TakeNeedWho;
	// leaveErr makes leaveLearner fail, for tests.
	needWho  bool
	leaveErr error

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

// Calm reports whether the player asked for calm effects.
func (c *Context) Calm() bool { return c.opts.Calm }

// SayWord says a word after a miss: the audio for entry in lang from the
// linked account's audio packs. It does nothing when the player turned
// it off, the game isn't linked, or there is no audio for the word.
func (c *Context) SayWord(lang string, entry words.Entry) {
	if !c.opts.SayWords() || c.Link == nil {
		return
	}
	if wav, ok := c.Link.Pronunciation(lang, entry); ok {
		c.Sound.Say(wav)
	}
}

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
	c.ShareWithAI()
	return nil
}

// ShareWithAI tells the AI which words it may be sent: the built-in
// lists' and the lists a teacher or parent sent or assigned, never the
// player's own (typed, imported or forged), nor what they added to a
// copy of a built-in list (#89). It is called whenever the lists or the
// AI change.
func (c *Context) ShareWithAI() {
	if c.AI == nil {
		return
	}
	starters, _ := StarterLists() // embedded, as the game ships them
	var share []words.Entry
	for _, l := range append(starters, c.Link.Lists()...) {
		share = append(share, l.Entries...)
	}
	c.AI.ShareOnly(share)
}

// ReloadSaves starts the game over on the files in the user's folder, as
// a start-up does: the learners, the learner playing, their link, word
// lists, profile and AI settings. It is used after progress moves in from
// another address (see PrepareMove), which replaces every learner's
// folder.
func (c *Context) ReloadSaves() error {
	c.Learners, c.needWho, c.addedID, c.adopting = nil, false, "", ""
	c.openLearners()
	c.openLink()
	c.linkSeen = c.Link.Changes()
	if err := c.LoadLists(); err != nil {
		return err
	}
	c.loadProfile()
	c.lockSettings()
	c.ApplyOptions()
	c.AI = c.loadAI()
	c.ShareWithAI()
	return nil
}

// PrepareMove is called before progress moves in from another address
// (move.Incoming.Apply). It ends the play session and closes the link, so
// nothing writes the folders that are about to be replaced. Then it
// unlinks every folder, the root and each learner's, whether or not the
// incoming list of learners names it: a folder's link may be to another
// learner than the one the list says (after an earlier import), and the
// tokens must not be left on disk for a folder that is replaced. Events
// still queued are sent first, and the server told after, in the
// background, best effort and bounded (the link's own timeouts). The game
// links again at its new address.
func (c *Context) PrepareMove() {
	c.EndSession()
	c.Link.Close()
	names, _ := save.Root.All()
	folders := []save.Folder{save.Root}
	seen := map[string]bool{}
	for _, n := range names {
		rest, ok := strings.CutPrefix(n, profile.ProfilesDir+"/")
		id, _, found := strings.Cut(rest, "/")
		if ok && found && !seen[id] {
			seen[id] = true
			folders = append(folders, save.Folder(profile.ProfilesDir+"/"+id))
		}
	}
	var wg sync.WaitGroup
	for _, f := range folders {
		if p := link.PeekFolder(f); p.Tokens || p.PendingSSO {
			wg.Add(1)
			go func() {
				defer wg.Done()
				l := link.Open(link.Options{Store: f, Version: Version, OwnDir: WordsDir})
				l.Close() // sends what is queued, within its timeouts
				l.Unlink()
			}()
		}
	}
	wg.Wait()
	for _, f := range folders {
		// Not exportable, so the move would leave them.
		for _, n := range [...]string{"link.json", "link-queue.json", "link-parked.json"} {
			f.Remove(n)
		}
	}
}

// loadProfile loads the profile of the learner playing.
func (c *Context) loadProfile() {
	prof, errs := profile.Load()
	c.Profile = prof
	if len(errs) > 0 {
		c.Notify("Some saved progress was damaged")
	}
}

// NewOutbox returns the report queue in the user's folder, sending to
// report.ServerURL(). Its reports are anonymous until SendReportsAsLink.
func NewOutbox() *report.Outbox {
	return &report.Outbox{
		Queue:  report.NewQueue(save.Root),
		Sender: &report.Sender{Server: report.ServerURL(), UserAgent: UserAgent()},
	}
}

// SendReportsAsLink makes o send reports with the linked game's access
// token, so the server knows which game sent them. Each report is tagged
// with the link it was made under and goes with a token only if that
// link is still the one playing when it is sent: a report written by one
// learner and sent after switching to another goes anonymously, never
// as the other learner. A report made while the game isn't linked is
// anonymous.
func (c *Context) SendReportsAsLink(o *report.Outbox) {
	if c.sendAs == nil {
		c.sendAs = &atomic.Pointer[link.Client]{}
	}
	c.sendAs.Store(c.Link)
	s := o.Sender
	o.From = func() string { return c.Link.DeviceID() } // on the game's goroutine
	s.Token = func(from string) string { return c.reportToken(s.Server, from) }
}

// reportToken is the access token of the link playing, or "" when it
// isn't linked, isn't the link from that a report was made under, or is
// linked to another server than server, the one reports go to: a token
// is only ever sent to the server that made it.
func (c *Context) reportToken(server, from string) string {
	l := c.sendAs.Load()
	if l == nil || from == "" || l.DeviceID() != from || strings.TrimRight(l.Server(), "/") != strings.TrimRight(server, "/") {
		return ""
	}
	return l.AccessToken()
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
	ctx.openLearners()
	ctx.openLink()
	watchPage(func() *link.Client { return ctx.Link }, ctx.SaveOnClose, ctx.SaveOnHide)
	if err := ctx.LoadLists(); err != nil {
		return nil, err
	}
	ctx.loadProfile()
	ctx.lockSettings()
	ctx.linkSeen = ctx.Link.Changes()
	ctx.ApplyOptions()
	ctx.AI = ctx.loadAI()
	ctx.ShareWithAI()
	if p := ctx.AI.Provider(); p != nil {
		ctx.AI.Check(p.ID) // free: it lists the models the key can use
	}
	ctx.Reports = NewOutbox()
	ctx.SendReportsAsLink(ctx.Reports)
	go ctx.Reports.Flush(context.Background()) // reports left from last time
	ctx.scenes.stack = []Scene{first(ctx)}
	return &Game{ctx: ctx}, nil
}

// Update implements ebiten.Game.
func (g *Game) Update() error {
	defer guard()
	if ebiten.IsWindowBeingClosed() {
		// The window's close button: main asked to handle it, so the
		// run can be saved before the game ends.
		g.ctx.SaveOnClose()
		return ebiten.Termination
	}
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
