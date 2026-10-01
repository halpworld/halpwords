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
// one to send it as later, and it is dropped. c.mu is held.
func (c *Client) park() {
	if c.q.len() == 0 || c.st.Me == nil || c.st.Me.Learner.ID == "" {
		return
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
	p.Queue = mergeQueues(p.Queue, c.q)
	c.saveParked()
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
// its sequence numbers.
func mergeQueues(a, b queue) queue {
	q := queue{
		Answers:  slices.Concat(a.Answers, b.Answers),
		Sessions: slices.Concat(a.Sessions, b.Sessions),
		Totals:   slices.Concat(a.Totals, b.Totals),
		Folded:   a.Folded + b.Folded,
		sending:  a.sending,
	}
	slices.SortStableFunc(q.Answers, func(x, y qAnswer) int { return cmp.Compare(x.Seq, y.Seq) })
	slices.SortStableFunc(q.Sessions, func(x, y qSession) int { return cmp.Compare(x.Seq, y.Seq) })
	slices.SortStableFunc(q.Totals, func(x, y qTotals) int { return cmp.Compare(x.Seq, y.Seq) })
	return q
}
