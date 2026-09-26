package game

import (
	"slices"
	"time"

	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/profile"
	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/pkg/settings"
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
	var roles []string
	for _, lang := range words.Languages {
		if ls, d := c.Link.Decide(lang, s.Own(lang), nil); d.Locked() {
			if s.Locked == nil {
				s.Locked = map[string]profile.LangSettings{}
			}
			s.Locked[lang.Code] = ls
			for _, r := range d.Roles() {
				if !slices.Contains(roles, r) {
					roles = append(roles, r)
				}
			}
		}
	}
	if s.Locked != nil {
		if len(roles) == 0 {
			if me := c.Link.Me(); me != nil {
				roles = me.SeenBy
			}
		}
		s.LockNote = link.SetByText(roles)
	}
}

// SettingsFor returns the settings to play lang with: the player's own,
// or what grown-ups set (Profile.Settings.For); for a quest, with its
// settings for its list too (link.Client.Decide). note says who set a
// quest's settings when they changed anything, such as "Set on the
// website by your teacher.", and is empty otherwise.
func (c *Context) SettingsFor(lang *words.Language, q *link.Quest) (ls profile.LangSettings, note string) {
	if c.Profile == nil {
		return profile.Preset(lang), ""
	}
	ls = c.Profile.Settings.For(lang)
	if q == nil {
		return ls, ""
	}
	got, d := c.Link.Decide(lang, c.Profile.Settings.Own(lang), q)
	if d.Accents.Source != settings.Assignment && d.Timer.Source != settings.Assignment {
		return ls, ""
	}
	return got, link.SetByText(d.Roles())
}

// QuestByID returns the quest with the id, as the server last sent it,
// or nil.
func (c *Context) QuestByID(id string) *link.Quest {
	for _, q := range c.Link.Quests() {
		if q.ID == id {
			return &q
		}
	}
	return nil
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
