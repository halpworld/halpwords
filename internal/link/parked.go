package link

import (
	"cmp"
	"encoding/json"
	"slices"
	"time"
)

// parkedFile keeps the events a game hadn't sent when the server stopped
// taking its tokens (lost), by learner, until that learner links the
// game again. It is private: it names the learner.
const parkedFile = "link-parked.json"

// parkedFor is how long kept events wait for their learner. Children's
// data: kept no longer than needed.
var parkedFor = 30 * 24 * time.Hour

// parkedAhead is how far in the future a parked entry may be dated (a
// clock a little off) before it is taken as expired: dated by a clock
// that later went back, it would otherwise never expire.
const parkedAhead = 24 * time.Hour

// parkedExpired reports whether p waited longer than parkedFor, or is
// dated more than parkedAhead in the future.
func parkedExpired(p parked, now time.Time) bool {
	return now.Sub(p.At) > parkedFor || p.At.Sub(now) > parkedAhead
}

// parked are the events not sent for one learner when the game lost its
// link, and when.
type parked struct {
	Learner string
	At      time.Time
	Queue   queue
}

// park keeps the queue for the linked learner before the link is
// forgotten. Without a learner (the server never said who), there is no
// one to send it as later, and it is dropped. c.mu is held. When the
// parked file can't be written, the events stay in memory, and Close
// tries again.
//
// Accepted: a batch the server took whose answer was lost, followed by
// the link being lost before the game sent it again, is parked and later
// sent once more after the learner links the game again, on a new device
// whose sequence numbers the server hasn't seen. Those few events are
// then counted twice. This is rare, and better than losing them.
func (c *Client) park() error {
	if c.q.len() == 0 || c.st.Me == nil || c.st.Me.Learner.ID == "" {
		return nil
	}
	c.expireParked()
	id := c.st.Me.Learner.ID
	i := slices.IndexFunc(c.parked, func(p parked) bool { return p.Learner == id })
	if i < 0 {
		c.parked = append(c.parked, parked{Learner: id})
		i = len(c.parked) - 1
	}
	p := &c.parked[i]
	p.At = c.now()
	// The queue's copy of an event is the newer one (totals grow).
	p.Queue = mergeQueues(c.q, p.Queue)
	p.Queue.sending = nil
	return c.writeParked()
}

// parkForUnlink parks the queue for the learner before unlink forgets
// them, so the events are never only in memory: a web game never gets to
// Close, and a tab that closes takes them with it. When the parked file
// can't be written (local storage is full), the queue file, which holds
// the same events, is removed to make room and parking is tried again. If
// that fails too, keepFiles is true: unlink leaves the queue file and the
// link's state file on disk, so a game started again is still linked as
// that learner, with the events queued, and loses the link again (its
// tokens are dead) with more room, if there is. Meanwhile the parked
// events are in memory, and retryParked (from Save, TrySave and Close)
// writes them as soon as it can, and then lets go of the old files. c.mu
// is held.
func (c *Client) parkForUnlink() (keepFiles bool) {
	q := c.q
	err := c.park()
	if err != nil && c.o.Store != nil && q.len() > 0 {
		c.o.Store.Remove(queueFile)
		if err = c.writeParked(); err != nil {
			// Put back what was removed (there is room for it). If
			// that fails too, the events are only in the parked events
			// in memory; the retries write those, and the state file
			// stays as it is (pending).
			data, merr := json.Marshal(q)
			if merr == nil {
				merr = c.o.Store.Write(queueFile, data)
			}
			_ = merr
		}
	}
	return err != nil
}

// retryParked writes the parked events again if they couldn't be written.
// Written, they let the old queue and state files go (parkForUnlink),
// which the caller does by writing the queue (wrote). It reports the error
// if they still can't be written. c.mu is held.
func (c *Client) retryParked() (wrote bool, err error) {
	if !c.parkedUnsaved {
		return false, nil
	}
	if err := c.writeParked(); err != nil {
		return false, err
	}
	if c.unlinkPending {
		c.unlinkPending = false
		// The queue file goes before the state is written unlinked: a
		// game that stops in between must not start unlinked with the
		// old queue.
		c.o.Store.Remove(queueFile)
		if !c.st.linked() {
			c.saveState()
		}
		// The queue file is written again if there is a queue now.
		c.dirty = true
		return true, nil
	}
	return false, nil
}

// releasePending is called before another link starts (or the game is
// unlinked): the old queue file must not stay on disk, or a game that
// stops before its next save would send the first learner's events as the
// next. The parked events are written if they can be; if not, they stay
// in memory, and removing the file needs no room. c.mu is held.
func (c *Client) releasePending() {
	if !c.unlinkPending || c.o.Store == nil {
		return
	}
	if _, err := c.retryParked(); err == nil && !c.unlinkPending {
		return
	}
	c.o.Store.Remove(queueFile)
	c.unlinkPending = false
}

// unpark adds the events kept for the learner to the queue, and reports
// whether there were any. c.mu is held; the caller saves the queue, then
// the parked events.
func (c *Client) unpark(learner string) bool {
	c.expireParked()
	i := slices.IndexFunc(c.parked, func(p parked) bool { return p.Learner == learner })
	if learner == "" || i < 0 {
		return false
	}
	c.q = mergeQueues(c.q, c.parked[i].Queue)
	c.parked = slices.Delete(c.parked, i, i+1)
	c.dirty = true
	c.fold()
	return true
}

// expireParked drops kept events older than parkedFor, and reports
// whether it dropped any. c.mu is held.
func (c *Client) expireParked() bool {
	now := c.now()
	n := len(c.parked)
	c.parked = slices.DeleteFunc(c.parked, func(p parked) bool { return parkedExpired(p, now) })
	return len(c.parked) != n
}

// writeParked saves the parked events, and remembers when that failed so
// Close tries again. c.mu is held.
func (c *Client) writeParked() error {
	err := c.saveParked()
	c.parkedUnsaved = err != nil
	return err
}

// dropQueued removes from the parked events those already in the queue
// (a game that stopped after writing the queue and before the parked
// file), and reports whether it removed any. c.mu is held.
func (c *Client) dropQueued() bool {
	seen := c.q.seqs()
	changed := false
	for i := range c.parked {
		var n bool
		c.parked[i].Queue, n = withoutSeqs(c.parked[i].Queue, seen)
		changed = changed || n
	}
	n := len(c.parked)
	c.parked = slices.DeleteFunc(c.parked, func(p parked) bool { return p.Queue.len() == 0 })
	return changed || len(c.parked) != n
}

// saveParked writes parkedFile, or removes it when nothing is kept. c.mu
// is held.
func (c *Client) saveParked() error {
	if c.o.Store == nil {
		return nil
	}
	if len(c.parked) == 0 {
		return c.o.Store.Remove(parkedFile)
	}
	data, err := json.Marshal(c.parked)
	if err != nil {
		return err
	}
	return c.o.Store.WritePrivate(parkedFile, data)
}

// maxParkedSeq is the highest sequence number of the kept events.
func maxParkedSeq(ps []parked) int64 {
	var m int64
	for _, p := range ps {
		m = max(m, p.Queue.maxSeq())
	}
	return m
}

// mergeQueues returns the events of a and b, each kind in the order of
// its sequence numbers. An event of b whose sequence number is already in
// a (the same event, kept twice) is left out: a's copy is kept.
func mergeQueues(a, b queue) queue {
	b, dup := withoutSeqs(b, a.seqs())
	folded := a.Folded + b.Folded
	if dup && b.len() == 0 {
		folded = a.Folded // b was all copies of a
	}
	q := queue{
		Answers:  slices.Concat(a.Answers, b.Answers),
		Sessions: slices.Concat(a.Sessions, b.Sessions),
		Totals:   slices.Concat(a.Totals, b.Totals),
		Folded:   folded,
		sending:  a.sending,
	}
	slices.SortStableFunc(q.Answers, func(x, y qAnswer) int { return cmp.Compare(x.Seq, y.Seq) })
	slices.SortStableFunc(q.Sessions, func(x, y qSession) int { return cmp.Compare(x.Seq, y.Seq) })
	slices.SortStableFunc(q.Totals, func(x, y qTotals) int { return cmp.Compare(x.Seq, y.Seq) })
	return q
}

// seqs are the sequence numbers of the queued events, of every kind (one
// counter numbers them all).
func (q *queue) seqs() map[int64]bool {
	m := make(map[int64]bool, q.len())
	for _, a := range q.Answers {
		m[a.Seq] = true
	}
	for _, s := range q.Sessions {
		m[s.Seq] = true
	}
	for _, t := range q.Totals {
		m[t.Seq] = true
	}
	return m
}

// withoutSeqs returns q without the events whose sequence numbers are in
// seen, and reports whether it left any out. q's slices aren't changed.
func withoutSeqs(q queue, seen map[int64]bool) (queue, bool) {
	n := q.len()
	q.Answers = slices.DeleteFunc(slices.Clone(q.Answers), func(a qAnswer) bool { return seen[a.Seq] })
	q.Sessions = slices.DeleteFunc(slices.Clone(q.Sessions), func(s qSession) bool { return seen[s.Seq] })
	q.Totals = slices.DeleteFunc(slices.Clone(q.Totals), func(t qTotals) bool { return seen[t.Seq] })
	return q, q.len() != n
}

// SweepParked drops the parked events in the folder f that waited longer
// than parkedFor for their learner. The game calls it for every learner's
// folder at start-up, so a learner who never plays again doesn't keep
// them for ever. A file that can't be parsed can never be sent, and is
// removed.
func SweepParked(f Store, now time.Time) error {
	data, err := f.Read(parkedFile)
	if err != nil {
		return nil // none (or unreadable: Open's business)
	}
	var ps []parked
	if json.Unmarshal(data, &ps) != nil {
		return f.Remove(parkedFile)
	}
	n := len(ps)
	ps = slices.DeleteFunc(ps, func(p parked) bool { return parkedExpired(p, now) })
	switch {
	case len(ps) == n:
		return nil
	case len(ps) == 0:
		return f.Remove(parkedFile)
	}
	if data, err = json.Marshal(ps); err != nil {
		return err
	}
	return f.WritePrivate(parkedFile, data)
}
