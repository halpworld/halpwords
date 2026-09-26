package game

import (
	"time"

	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/profile"
	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/pkg/words"
)

// Remove deletes a file from the user's folder, for the link.
func (saveStore) Remove(name string) error { return save.Remove(name) }

// openLink opens the link to a grown-up's account, if there is one, and
// starts syncing in the background. A game that was never linked does
// nothing on the network.
func (c *Context) openLink() {
	c.Link = link.Open(link.Options{Store: saveStore{}, Version: Version, OwnDir: WordsDir})
	c.Link.Start()
	watchPage(c.Link)
}

// pollLink picks up what the link brought in the background: assigned
// lists, the word memory from the server and settings a grown-up set. It
// never waits for the network.
func (c *Context) pollLink() {
	n := c.Link.Changes()
	if n == c.linkSeen {
		return
	}
	c.linkSeen = n
	c.LoadLists()
	if c.Profile != nil && c.Link.MergeMemory(c.Profile.Memory) {
		c.Profile.SaveMemory()
	}
	c.lockSettings()
}

// lockSettings puts the settings a grown-up set into the profile, where
// Settings shows them locked.
func (c *Context) lockSettings() {
	if c.Profile == nil {
		return
	}
	s := &c.Profile.Settings
	s.Locked, s.LockNote = nil, ""
	for _, lang := range words.Languages {
		if ls, ok := c.Link.Locked(lang, s.Own(lang)); ok {
			if s.Locked == nil {
				s.Locked = map[string]profile.LangSettings{}
			}
			s.Locked[lang.Code] = ls
		}
	}
	if s.Locked != nil {
		var roles []string
		if me := c.Link.Me(); me != nil {
			roles = me.SeenBy
		}
		s.LockNote = link.SetByText(roles)
	}
}

// session is the play session going on, sent to the grown-up's account
// when it ends.
type session struct {
	start time.Time
	mode  string
	lang  string
	floor func() int
}

// Playing starts a play session in mode ("practice", "adventure" or
// "hardcore") and lang, unless one is going on: scenes call it every tick.
// A session in another mode or language ends first. floor, when not nil,
// says the deepest floor reached. It does nothing when the game isn't
// linked.
func (c *Context) Playing(mode, lang string, floor func() int) {
	if s := c.session; s != nil && s.mode == mode && s.lang == lang {
		return
	}
	c.EndSession()
	if !c.Link.Linked() {
		return
	}
	c.session = &session{start: time.Now(), mode: mode, lang: lang, floor: floor}
}

// EndSession ends the play session going on, if there is one.
func (c *Context) EndSession() {
	s := c.session
	if s == nil {
		return
	}
	c.session = nil
	fl := 0
	if s.floor != nil {
		fl = s.floor()
	}
	c.Link.Session(link.Session{Start: s.start, Mode: s.mode, Lang: s.lang,
		Secs: int(time.Since(s.start).Seconds()), Floor: fl})
}

// Close ends the play session and sends what the link has waiting, for a
// moment. The game calls it when it quits.
func (g *Game) Close() {
	g.ctx.EndSession()
	g.ctx.Link.Close()
}
