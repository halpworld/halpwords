package game

import (
	"errors"
	"slices"

	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/profile"
	"github.com/halpworld/halpwords/internal/save"
)

// openLearners reads the learners who play on this computer, moving the
// files of a game from before into the first one's folder, and makes the
// last one who played current. If that fails, the game plays with the
// user's folder as before.
func (c *Context) openLearners() {
	ls, err := profile.OpenLearners(save.Root)
	if err == nil {
		err = ls.Use(ls.Current)
	}
	if err != nil {
		c.Notify("Couldn't read who plays here")
		save.Use(save.Root)
		return
	}
	c.Learners = ls
}

// Learner is the learner playing now, or nil when the game plays without
// a list of learners.
func (c *Context) Learner() *profile.Learner {
	if c.Learners == nil {
		return nil
	}
	return c.Learners.CurrentLearner()
}

// SwitchLearner makes another learner the one playing: it ends the play
// session, closes the link of the learner before (in the background: it
// keeps writing to their folder only), and loads the new learner's
// lists, profile and link.
func (c *Context) SwitchLearner(id string) error {
	if c.Learners == nil {
		return errors.New("no learners")
	}
	if c.Learners.Find(id) == nil {
		return errors.New("no such learner")
	}
	c.EndSession()
	if old := c.Link; old != nil {
		c.closeLink(save.Current(), old)
	}
	if err := c.Learners.Use(id); err != nil {
		return err
	}
	c.waitClosed(save.Current())
	c.openLink()
	c.linkSeen = c.Link.Changes()
	if err := c.LoadLists(); err != nil {
		return err
	}
	c.loadProfile()
	c.lockSettings()
	c.ApplyOptions()
	return nil
}

// closeLink closes a learner's link in the background.
func (c *Context) closeLink(f save.Folder, l *link.Client) {
	if c.closing == nil {
		c.closing = map[save.Folder]chan struct{}{}
	}
	done := make(chan struct{})
	c.closing[f] = done
	go func() {
		l.Close()
		close(done)
	}()
}

// waitClosed waits for the link of the learner in folder f to finish
// closing, if it is, so two links never write the same files.
func (c *Context) waitClosed(f save.Folder) {
	if done, ok := c.closing[f]; ok {
		<-done
		delete(c.closing, f)
	}
}

// AddLearner adds a learner with an empty folder and switches to them.
// name may be empty: they are "Player N" until they sign in.
func (c *Context) AddLearner(name string) (*profile.Learner, error) {
	if c.Learners == nil {
		return nil, errors.New("no learners")
	}
	l, err := c.Learners.Add(name)
	if err != nil {
		return nil, err
	}
	if err := c.SwitchLearner(l.ID); err != nil {
		return nil, err
	}
	return l, nil
}

// RemoveLearner deletes a learner's folder, with everything in it, and
// takes them off the list. Removing the learner playing switches to the
// one who played most recently before, or to a new, empty learner.
func (c *Context) RemoveLearner(id string) error {
	if c.Learners == nil {
		return errors.New("no learners")
	}
	l := c.Learners.Find(id)
	if l == nil {
		return nil
	}
	if id == c.Learners.Current {
		c.EndSession()
		c.Link.Close() // it may still write its files: wait
		c.Link = nil
	}
	c.waitClosed(l.Folder())
	if err := c.Learners.Remove(id); err != nil {
		return err
	}
	if c.Learners.Current != "" {
		return c.Learners.Save()
	}
	next := c.lastUsed()
	if next == nil {
		var err error
		if next, err = c.Learners.Add(""); err != nil {
			return err
		}
	}
	return c.SwitchLearner(next.ID)
}

// lastUsed is the learner who played most recently, or nil.
func (c *Context) lastUsed() *profile.Learner {
	if len(c.Learners.List) == 0 {
		return nil
	}
	return slices.MaxFunc(c.Learners.List, func(a, b *profile.Learner) int { return a.LastUsed.Compare(b.LastUsed) })
}

// SignedIn is called once the learner playing has signed in: it keeps a
// lock on their folder (lock may be nil, as for a grown-up's pairing
// code) so only the same sign-in opens it again.
func (c *Context) SignedIn(lock *profile.Lock) {
	l := c.Learner()
	if l == nil {
		return
	}
	if lock != nil {
		l.Lock = lock
	}
	c.Learners.Save()
}

// SignOut signs the learner playing out: the game forgets their tokens
// and tells the server. If the school allows it (or a grown-up linked
// the game at home), their progress stays here, under their lock;
// otherwise their folder is deleted, and the game switches to another
// learner. done reports, from the game loop, when it has finished: the
// link needs a moment to close first.
func (c *Context) SignOut() (done func() bool) {
	keep := c.Link.KeepOnSignOut()
	l := c.Learner()
	c.EndSession()
	c.Link.Unlink()
	if keep || l == nil {
		if l != nil {
			l.LearnerID = ""
			c.Learners.Save()
		}
		return func() bool { return true }
	}
	old := c.Link
	closed := make(chan struct{})
	go func() {
		old.Close()
		close(closed)
	}()
	finished := false
	return func() bool {
		if finished {
			return true
		}
		select {
		case <-closed:
		default:
			return false
		}
		finished = true
		c.Link = old // closed: RemoveLearner closes it again, quickly
		if err := c.RemoveLearner(l.ID); err != nil {
			c.Notify("Couldn't remove your progress here")
		}
		return true
	}
}

// notePlayer keeps the linked learner's name and ID on the list of
// learners, for the Switch learner screen.
func (c *Context) notePlayer() {
	l := c.Learner()
	me := c.Link.Me()
	if l == nil || me == nil || !c.Link.Linked() {
		return
	}
	name, id := me.Learner.DisplayName, me.Learner.ID
	if name == "" || (l.Name == name && l.LearnerID == id) {
		return
	}
	l.Name, l.LearnerID = name, id
	c.Learners.Save()
}
