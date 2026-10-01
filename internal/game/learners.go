package game

import (
	"errors"
	"slices"
	"strings"

	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/profile"
	"github.com/halpworld/halpwords/internal/save"
)

// openLearners reads the learners who play on this computer, moving the
// files of a game from before into the first one's folder, and makes the
// last one who played current, unless they have to sign in first: then a
// guest plays and the Switch learner screen comes first, so the next
// child at a shared computer never starts in a classmate's progress. A
// grown-up's pairing-code learner at home resumes as before. If this
// fails, the game plays with the user's folder as before.
func (c *Context) openLearners() {
	ls, err := profile.OpenLearners(save.Root)
	if err != nil {
		c.Notify("Couldn't read who plays here")
		save.Use(save.Root)
		return
	}
	c.Learners = ls
	if ls.Repaired {
		// The locks were lost: whoever holds tokens must sign in again.
		for _, l := range ls.List {
			if link.PeekFolder(l.Folder()).Tokens {
				l.NeedsSignIn = true
			}
		}
		ls.Save()
		c.Notify("Learner data was repaired: sign in again to open a learner")
	}
	id := ls.Current
	if cur := ls.CurrentLearner(); cur != nil && c.NeedsSignIn(cur) {
		g, err := c.guest()
		if err != nil {
			c.Notify("Couldn't read who plays here")
			c.Learners = nil
			save.Use(save.Root)
			return
		}
		id, c.needWho = g.ID, true
	}
	if err := ls.Use(id); err != nil {
		c.Notify("Couldn't read who plays here")
		c.Learners = nil
		save.Use(save.Root)
	}
}

// TakeNeedWho reports, once, that the game started on a guest because the
// learner who played last has to sign in: the title shows who is playing.
func (c *Context) TakeNeedWho() bool {
	n := c.needWho
	c.needWho = false
	return n
}

// schoolTokens reports whether a learner's folder holds tokens from a
// sign-in at school, or a school account sign-in on its way.
func schoolTokens(l *profile.Learner) bool {
	p := link.PeekFolder(l.Folder())
	return p.PendingSSO || p.Tokens && p.School()
}

// NeedsSignIn reports whether a learner can be opened only by signing in:
// they have a lock (opened with UnlockLearner), or signed in at school
// without one (a school account), or their folder holds tokens from a
// school sign-in. A learner like that is never played as by choosing
// them from the list. A grown-up's pairing-code learner has none of
// these: it is the home case.
func (c *Context) NeedsSignIn(l *profile.Learner) bool {
	return l.Lock != nil || l.NeedsSignIn || schoolTokens(l)
}

// hasTokens reports whether a learner's folder holds a sign-in, or one
// that is on its way. LearnerID says nothing: it is filled in only once
// the game has heard from the server.
func hasTokens(l *profile.Learner) bool {
	p := link.PeekFolder(l.Folder())
	return p.Tokens || p.PendingSSO
}

// Pristine reports whether a learner is a guest nobody has played as:
// no lock, no sign-in, no files at all in their folder, and a name the
// game gave. Only a guest like that is safe for the next child.
func (c *Context) Pristine(l *profile.Learner) bool {
	if l.Lock != nil || l.NeedsSignIn || l.LearnerID != "" || hasTokens(l) {
		return false
	}
	if l.Name != "" && !strings.HasPrefix(l.Name, "Player ") {
		return false
	}
	names, err := l.Folder().All()
	return err == nil && len(names) == 0
}

// Learner is the learner playing now, or nil when the game plays without
// a list of learners.
func (c *Context) Learner() *profile.Learner {
	if c.Learners == nil {
		return nil
	}
	return c.Learners.CurrentLearner()
}

// ErrLearnerLocked is switching to a learner who has to sign in first:
// their folder has a lock, or they signed in at school.
var ErrLearnerLocked = errors.New("this learner has to sign in first")

// SwitchLearner makes another learner the one playing: it ends the play
// session, closes the link of the learner before (in the background: it
// keeps writing to their folder only), and loads the new learner's
// lists, profile and link. A learner who has to sign in first (see
// NeedsSignIn) opens only with UnlockLearner or a new sign-in: on a
// shared computer the next child must never play as, and send answers
// for, a classmate.
func (c *Context) SwitchLearner(id string) error {
	if c.Learners == nil {
		return errors.New("no learners")
	}
	l := c.Learners.Find(id)
	if l == nil {
		return errors.New("no such learner")
	}
	if id != c.Learners.Current && c.NeedsSignIn(l) {
		return ErrLearnerLocked
	}
	return c.switchLearner(id)
}

// UnlockLearner tries secret on the learner's lock (see
// profile.Learners.Unlock), and switches to them if it opens.
func (c *Context) UnlockLearner(id, secret string) (bool, error) {
	if c.Learners == nil {
		return false, errors.New("no learners")
	}
	l := c.Learners.Find(id)
	if l == nil {
		return false, errors.New("no such learner")
	}
	if l.Lock == nil && id != c.Learners.Current && c.NeedsSignIn(l) {
		return false, ErrLearnerLocked // nothing local to open it with
	}
	ok, err := c.Learners.Unlock(id, secret)
	if !ok {
		return false, err
	}
	return true, c.switchLearner(id)
}

// switchLearner switches without asking for a lock: the callers have
// opened it, or the learner is playing already.
func (c *Context) switchLearner(id string) error {
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
	prev := c.Learners.Current
	if err := c.switchLearner(l.ID); err != nil {
		return nil, err
	}
	c.addedFrom = prev
	return l, nil
}

// CancelAddLearner takes back the learner AddLearner added and goes back
// to the one who played before: they were open already, even with a lock.
func (c *Context) CancelAddLearner() error {
	prev := c.addedFrom
	c.addedFrom = ""
	added := c.Learners.Current
	if prev == "" || prev == added || c.Learners.Find(prev) == nil {
		return nil
	}
	if err := c.switchLearner(prev); err != nil {
		return err
	}
	return c.RemoveLearner(added)
}

// RemoveLearner deletes a learner's folder, with everything in it, and
// takes them off the list. Removing the learner playing switches to the
// one who played most recently before who has no lock and isn't linked
// to an account, or to a new, empty learner: never to a learner with a
// lock, whom nobody has opened.
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
	return c.leaveLearner()
}

// leaveLearner switches to a guest anyone may play as: see guest. If it
// can't, the game must not stay on the learner who is leaving: it fails
// closed, with nobody playing and the Switch learner screen next.
func (c *Context) leaveLearner() error {
	err := c.leaveErr
	var g *profile.Learner
	if err == nil {
		if g, err = c.guest(); err == nil {
			err = c.switchLearner(g.ID)
		}
	}
	if err != nil {
		c.failClosed()
	}
	return err
}

// guest is a learner anyone may play as: the most recent one who is
// pristine (see Pristine), so guests don't pile up, or a new, empty one.
// Never someone with a lock, an account, a school sign-in or progress.
func (c *Context) guest() (*profile.Learner, error) {
	var open []*profile.Learner
	for _, l := range c.Learners.List {
		if c.Pristine(l) {
			open = append(open, l)
		}
	}
	if len(open) > 0 {
		return slices.MaxFunc(open, func(a, b *profile.Learner) int { return a.LastUsed.Compare(b.LastUsed) }), nil
	}
	l, err := c.Learners.Add("")
	if err != nil {
		return nil, err
	}
	return l, c.Learners.Save()
}

// sinkFolder is where the game plays when nobody does, after a switch
// failed: it isn't a learner's, so nobody's progress is in it.
const sinkFolder = save.Folder(profile.ProfilesDir + "/_none")

// failClosed leaves nobody as the learner playing. The title sees that
// and goes to the Switch learner screen before any play.
func (c *Context) failClosed() {
	c.EndSession()
	if c.Link != nil {
		c.closeLink(save.Current(), c.Link)
	}
	c.Learners.Current = ""
	c.Learners.Save()
	save.Use(sinkFolder)
	c.waitClosed(sinkFolder)
	c.openLink()
	c.linkSeen = c.Link.Changes()
	c.LoadLists()
	c.loadProfile()
	c.lockSettings()
	c.ApplyOptions()
	c.Notify("Couldn't switch learners: choose who is playing")
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
	if lock == nil && link.IsSchoolWay(c.Link.Way()) {
		l.NeedsSignIn = true // a school account: nothing local opens it
	}
	c.Learners.Save()
}

// SignOut signs the learner playing out: the game forgets their tokens
// and tells the server. If the school allows it (or a grown-up linked
// the game at home), their progress stays here, under their lock;
// otherwise their folder is deleted. After a sign-out at school, or of a
// learner with a lock, the game switches to a guest anyone may play as,
// as it does when a learner is removed, so nobody plays on as them,
// unlocked. done reports, from the game loop, when it has finished: the
// link needs a moment to close first.
func (c *Context) SignOut() (done func() bool) {
	school := link.IsSchoolWay(c.Link.Way())
	keep := c.Link.KeepOnSignOut()
	l := c.Learner()
	c.EndSession()
	c.Link.Unlink()
	if keep || l == nil {
		if l != nil {
			l.LearnerID = ""
			if school && l.Lock == nil {
				l.NeedsSignIn = true // the tokens are gone: nothing else says so
			}
			c.Learners.Save()
			if school || l.Lock != nil {
				if err := c.leaveLearner(); err != nil {
					c.Notify("Couldn't switch to another learner")
				}
			}
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
