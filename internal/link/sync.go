package link

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Link links the game with a pairing code from the website, in the
// background: Status says when it is done (Busy goes false, and Linked
// or Err is set). The first sync follows at once.
func (c *Client) Link(code string) {
	c.mu.Lock()
	c.busy, c.err = true, nil
	c.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
		defer cancel()
		if err := c.LinkNow(ctx, code); err == nil {
			c.Sync(ctx)
		}
	}()
}

// LinkNow links the game with a pairing code and waits for the answer.
func (c *Client) LinkNow(ctx context.Context, code string) error {
	c.syncMu.Lock()
	defer c.syncMu.Unlock()
	c.mu.Lock()
	linked := c.st.linked()
	c.busy = true
	c.mu.Unlock()
	err := ErrLinked
	if !linked {
		err = c.link(ctx, code)
	}
	c.mu.Lock()
	c.busy, c.err = false, err
	c.mu.Unlock()
	return err
}

func (c *Client) link(ctx context.Context, code string) error {
	code = strings.TrimSpace(code)
	if n := len(strings.NewReplacer("-", "", " ", "").Replace(code)); n != 8 || len(code) > 16 {
		return ErrBadCode
	}
	var t tokens
	// Sent once: a code works once.
	payload, _ := json.Marshal(map[string]string{"code": code, "name": c.deviceName()})
	_, _, err := c.once(ctx, http.MethodPost, "/api/v1/link", "", nil, payload, &t)
	var e *Error
	if errors.As(err, &e) {
		switch {
		case e.Code == codeInvalidCode || (e.Status == http.StatusBadRequest && e.Code == codeInvalidRequest):
			return ErrBadCode
		case e.Code == codeNotLinkable || e.Status == http.StatusForbidden:
			return ErrNotLinkable
		}
	}
	if err != nil {
		return err
	}
	if t.AccessToken == "" || t.RefreshToken == "" {
		return errors.New("link: the server sent no tokens")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	c.st = state{NextSeq: c.st.NextSeq, LinkedAt: c.now()}
	c.q = queue{}
	c.dirty = true
	c.keep(t)
	c.failures, c.note = 0, ""
	c.loadLists()
	c.changes++
	return nil
}

// Sync sends the queued events and fetches the learner, the assigned
// lists, the quests and the word memory, waiting for the answers. A
// game that isn't linked does nothing. The background loop calls it;
// tests and quitting call it directly.
func (c *Client) Sync(ctx context.Context) error {
	c.syncMu.Lock()
	defer c.syncMu.Unlock()
	c.mu.Lock()
	if !c.st.linked() {
		c.mu.Unlock()
		return ErrNotLinked
	}
	gen := c.gen
	c.busy = true
	c.mu.Unlock()

	err := c.syncAll(ctx, gen)

	c.mu.Lock()
	c.busy = false
	switch {
	case err == nil:
		c.err, c.failures = nil, 0
		c.st.LastSync = c.now()
		c.saveState()
	case errors.Is(err, ErrNotLinked), errors.Is(err, ErrUnlinked):
		// unlinked while syncing, here or on the website: Note says so
	default:
		c.err = err
		c.failures++
	}
	c.mu.Unlock()
	c.saveQueue()
	return err
}

func (c *Client) syncAll(ctx context.Context, gen int) error {
	if err := c.syncMe(ctx, gen); err != nil {
		return err
	}
	if err := c.upload(ctx, gen); err != nil {
		return err
	}
	if err := c.syncLists(ctx, gen); err != nil {
		return err
	}
	if err := c.syncQuests(ctx, gen); err != nil {
		return err
	}
	return c.syncMemory(ctx, gen)
}

// syncMe fetches the learner. syncMu is held.
func (c *Client) syncMe(ctx context.Context, gen int) error {
	var me Me
	if _, _, err := c.authed(ctx, gen, http.MethodGet, "/api/v1/me", nil, nil, &me); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gen != gen {
		return ErrNotLinked
	}
	old, _ := json.Marshal(c.st.Me)
	now, _ := json.Marshal(&me)
	if string(old) != string(now) {
		c.st.Me = &me
		c.changes++
		return c.saveState()
	}
	return nil
}

// maxBatches is how many batches one sync sends at most, so a sync
// with a full queue still ends; the next one sends more.
const maxBatches = 60

// upload sends the queued events in batches, oldest first, until the
// queue is empty. syncMu is held.
func (c *Client) upload(ctx context.Context, gen int) error {
	for range maxBatches {
		c.mu.Lock()
		if c.gen != gen {
			c.mu.Unlock()
			return ErrNotLinked
		}
		if c.q.len() == 0 {
			c.mu.Unlock()
			return nil
		}
		b, seqs := c.takeBatch(c.batch)
		c.mu.Unlock()

		var res struct {
			LastSeq    int64 `json:"last_seq"`
			Stored     int   `json:"stored"`
			Duplicates int   `json:"duplicates"`
			Refused    []struct {
				Seq  int64  `json:"seq"`
				Code string `json:"code"`
			} `json:"refused"`
		}
		_, _, err := c.authed(ctx, gen, http.MethodPost, "/api/v1/events", nil, b, &res)

		c.mu.Lock()
		c.q.sending = nil
		if c.gen != gen {
			c.mu.Unlock()
			return ErrNotLinked
		}
		var e *Error
		switch {
		case err == nil:
			// Every event of the batch is handled, stored or refused.
			sent := map[int64]bool{}
			for _, s := range seqs {
				sent[s] = true
			}
			c.drop(sent)
			c.uploads++
			// A lost queue file must not make new events look like
			// ones the server already has.
			c.st.NextSeq = max(c.st.NextSeq, res.LastSeq+1)
			c.batch = min(MaxBatch, c.batch*2)
		case errors.As(err, &e) && e.Status == http.StatusRequestEntityTooLarge:
			if c.batch == 1 {
				c.drop(map[int64]bool{seqs[0]: true}) // one event too large: never sendable
			}
			c.batch = max(1, c.batch/2)
		case errors.As(err, &e) && e.Code == codeInvalidRequest:
			// The server will never take this batch as it is: drop the
			// events it names, or the whole batch if it names none, so
			// the queue never sticks.
			c.drop(badEvents(b, seqs, e))
		default:
			c.mu.Unlock()
			return err
		}
		c.mu.Unlock()
	}
	return nil
}

// badEvents are the events of a batch an invalid_request answer names
// (by paths such as /answers/3/tier), or all of them.
func badEvents(b wireBatch, seqs []int64, e *Error) map[int64]bool {
	out := map[int64]bool{}
	offset := map[string]int{"answers": 0, "sessions": len(b.Answers), "totals": len(b.Answers) + len(b.Sessions)}
	size := map[string]int{"answers": len(b.Answers), "sessions": len(b.Sessions), "totals": len(b.Totals)}
	for _, d := range e.Details {
		parts := strings.Split(strings.TrimPrefix(d.Path, "/"), "/")
		if len(parts) < 2 {
			continue
		}
		i, err := strconv.Atoi(parts[1])
		if _, ok := offset[parts[0]]; !ok || err != nil || i < 0 || i >= size[parts[0]] {
			continue
		}
		out[seqs[offset[parts[0]]+i]] = true
	}
	if len(out) == 0 {
		for _, s := range seqs {
			out[s] = true
		}
	}
	return out
}

// Start syncs in the background: now, every SyncEvery, and when SyncNow
// asks. While the server can't be reached it waits longer and longer
// between tries, up to half an hour. It also saves the queue every few
// seconds while answers come in.
func (c *Client) Start() {
	c.mu.Lock()
	if c.stop != nil {
		c.mu.Unlock()
		return
	}
	c.stop, c.stopped = make(chan struct{}), make(chan struct{})
	stop, stopped := c.stop, c.stopped
	c.mu.Unlock()
	go func() {
		defer close(stopped)
		save := time.NewTicker(saveEvery)
		defer save.Stop()
		next := time.NewTimer(0)
		defer next.Stop()
		for {
			select {
			case <-stop:
				return
			case <-save.C:
				c.saveQueue()
				continue
			case <-next.C:
			case <-c.kick:
			}
			ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
			c.Sync(ctx)
			cancel()
			next.Stop()
			select {
			case <-next.C:
			default:
			}
			next.Reset(c.wait())
		}
	}()
}

// wait is how long to wait before the next sync.
func (c *Client) wait() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	d := SyncEvery
	for i := 0; i < c.failures && d < maxBackoff; i++ {
		d *= 2
	}
	return min(d, maxBackoff)
}

// SyncNow asks for a sync soon, in the background.
func (c *Client) SyncNow() {
	if c == nil {
		return
	}
	c.mu.Lock()
	started := c.stop != nil
	c.mu.Unlock()
	if !started {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
			defer cancel()
			c.Sync(ctx)
		}()
		return
	}
	select {
	case c.kick <- struct{}{}:
	default:
	}
}

// Close stops the background sync, tries for a moment to send what is
// queued, and saves the queue. The game calls it when it quits.
func (c *Client) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	stop, stopped := c.stop, c.stopped
	c.stop = nil
	linked, pending := c.st.linked(), c.q.len()
	c.mu.Unlock()
	if stop != nil {
		close(stop)
	}
	if linked && pending > 0 {
		// A sync still running holds syncMu; don't wait for it.
		if c.syncMu.TryLock() {
			ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
			c.mu.Lock()
			gen := c.gen
			c.mu.Unlock()
			c.upload(ctx, gen)
			cancel()
			c.syncMu.Unlock()
		}
	}
	c.saveQueue()
	if stopped != nil {
		select {
		case <-stopped:
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// Unlink unlinks the game. It keeps the player's progress and the
// assigned lists, as their own lists (licensed lists are removed), and
// forgets the tokens and the events not yet sent. It doesn't wait for the
// network; a sync still running throws away what it gets. The family
// sees the game on the website until they remove it there.
func (c *Client) Unlink() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.unlink()
}

// lost unlinks the game because the server no longer takes its tokens.
func (c *Client) lost(gen int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gen != gen || !c.st.linked() {
		return
	}
	c.unlink()
	c.note = "This game was unlinked on the website. Your progress is still here."
}

// unlink does Unlink. c.mu is held.
func (c *Client) unlink() {
	if c.o.Store != nil {
		for _, li := range c.st.Lists {
			c.moveOut(li)
		}
	}
	c.gen++
	c.st = state{NextSeq: c.st.NextSeq}
	c.q = queue{}
	c.dirty = false
	if c.o.Store != nil {
		c.o.Store.Remove(queueFile)
	}
	c.memories = map[string]*fetched{}
	c.err, c.failures, c.note = nil, 0, ""
	c.loadLists()
	c.saveState()
	c.changes++
}
