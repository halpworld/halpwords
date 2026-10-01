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
	c.parked = slices.DeleteFunc(c.parked, func(p parked) bool { return now.Sub(p.At) > parkedFor })
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
// them for ever. A damaged file is left for Open, which starts afresh.
func SweepParked(f Store, now time.Time) error {
	data, err := f.Read(parkedFile)
	if err != nil {
		return nil // none (or unreadable: Open's business)
	}
	var ps []parked
	if json.Unmarshal(data, &ps) != nil {
		return nil
	}
	n := len(ps)
	ps = slices.DeleteFunc(ps, func(p parked) bool { return now.Sub(p.At) > parkedFor })
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
