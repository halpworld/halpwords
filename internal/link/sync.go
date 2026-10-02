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

// Link links the game with a pairing code from the website, or a login
// card's code, in the background: Status says when it is done (Signing goes
// false, and Linked or Err is set). The first sync follows at once, and
// goes on in the background.
func (c *Client) Link(code string) { c.SignIn(SignIn{Code: code}) }

// SignIn signs a learner in, in the background, as Link does: with a
// pairing code, a login card, or a class code and pictures.
func (c *Client) SignIn(s SignIn) {
	ctx, cancel := context.WithTimeout(c.life, syncTimeout)
	c.mu.Lock()
	c.busy, c.signing, c.err = true, true, nil
	c.signSeq++
	seq := c.signSeq
	c.mu.Unlock()
	go func() {
		defer cancel()
		if err := c.signInNow(ctx, s, seq); err == nil {
			c.Sync(ctx)
		}
	}()
}

// errSignInCancelled is the end of a sign-in the player gave up.
var errSignInCancelled = errors.New("link: sign-in cancelled")

// CancelSignIn gives up the sign-in SignIn has on its way (the player
// pressed Esc): Status stops saying Signing, and tokens that come back
// are not kept but revoked on the server. Its request is left to finish
// (bounded by the request timeout) and not cancelled, because a server
// that has already made tokens for it can only be told about them when
// they arrive. It returns false when there is no such sign-in or it has
// finished already (the game is linked), which then stands.
func (c *Client) CancelSignIn() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.signing || c.st.linked() {
		return false
	}
	c.signSeq++
	c.busy, c.signing = false, false
	return true
}

// LinkNow links the game with a pairing code or a login card's code and
// waits for the answer.
func (c *Client) LinkNow(ctx context.Context, code string) error {
	return c.SignInNow(ctx, SignIn{Code: code})
}

// SignInNow signs a learner in and waits for the answer.
func (c *Client) SignInNow(ctx context.Context, s SignIn) error {
	return c.signInNow(ctx, s, -1)
}

// signInNow is SignInNow for the sign-in numbered seq (see SignIn), or -1
// for one that can't be cancelled. One that was cancelled leaves Status
// alone: another sign-in may be on its way by now.
func (c *Client) signInNow(ctx context.Context, s SignIn, seq int) error {
	c.syncMu.Lock()
	defer c.syncMu.Unlock()
	c.mu.Lock()
	linked := c.st.linked()
	c.busy, c.signing = true, true
	c.mu.Unlock()
	err := ErrLinked
	if !linked {
		err = c.link(ctx, s, seq)
	}
	c.mu.Lock()
	if seq < 0 || seq == c.signSeq {
		c.busy, c.signing, c.err = false, false, err
	}
	c.mu.Unlock()
	return err
}

func (c *Client) link(ctx context.Context, s SignIn, seq int) error {
	body, err := s.body()
	if err != nil {
		return err
	}
	body["name"] = c.deviceName()
	var t tokens
	// Sent once: a pairing code works once, and a wrong card or picture
	// counts towards a lockout.
	payload, _ := json.Marshal(body)
	_, _, err = c.once(ctx, http.MethodPost, "/api/v1/link", "", nil, payload, &t)
	if err = s.explain(err); err != nil {
		return err
	}
	if t.AccessToken == "" || t.RefreshToken == "" {
		return errors.New("link: the server sent no tokens")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if seq >= 0 && seq != c.signSeq {
		// Given up (Esc) while the answer was on its way: the tokens are
		// dropped, and the server told.
		c.revoke(t)
		return errSignInCancelled
	}
	c.signedIn(t, s.Way())
	return nil
}

// signedIn keeps the tokens of a new sign-in made the way way. c.mu is
// held.
func (c *Client) signedIn(t tokens, way string) {
	c.gen++
	c.st = state{NextSeq: c.st.NextSeq, LinkedAt: c.now(), Way: way}
	c.q = queue{}
	c.dirty = true
	// Used even if it can't be saved: the code is spent, and the game
	// works until it quits, which tries to write them again.
	c.setTokens(t)
	c.stateUnsaved = c.saveState() != nil
	c.failures, c.note = 0, ""
	c.loadLists()
	c.changes++
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
	c.syncs++
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
	// The learner and the upload are gates: they cover the link itself
	// and the core data.
	if err := c.syncMe(ctx, gen); err != nil {
		return err
	}
	if err := c.upload(ctx, gen); err != nil {
		return err
	}
	// The rest don't depend on each other: one that fails (a bad list,
	// an endpoint with a problem) doesn't hold back the others, which
	// would leave finished runs unsent until the server refuses them as
	// stale. The first error is reported. Rankings come late, so a
	// server without them never holds back the rest, and audio packs
	// last of all: they are the biggest downloads.
	var first error
	for _, stage := range []func(context.Context, int) error{
		c.syncLists, c.syncQuests, c.syncMemory, c.uploadRuns, c.syncRanks, c.syncAudio,
	} {
		err := stage(ctx, gen)
		if err == nil {
			continue
		}
		if first == nil {
			first = err
		}
		if stopsSync(ctx, err) {
			if errors.Is(err, ErrNotLinked) || errors.Is(err, ErrUnlinked) {
				first = err // Sync treats an unlink differently from a failure
			}
			break
		}
	}
	return first
}

// stopsSync reports whether err (of a stage that may fail on its own) is
// one that makes the stages after it pointless: the game is unlinked,
// its tokens are refused, or the sync ran out of time.
func stopsSync(ctx context.Context, err error) bool {
	var e *Error
	return errors.Is(err, ErrNotLinked) || errors.Is(err, ErrUnlinked) || ctx.Err() != nil ||
		(errors.As(err, &e) && e.Status == http.StatusUnauthorized)
}

// syncMe fetches the learner. syncMu is held.
func (c *Client) syncMe(ctx context.Context, gen int) error {
	// LastSeq is the highest sequence number the server has from this
	// device; older servers leave it out. It isn't kept in Me, so it
	// doesn't count as a change to the learner.
	var got struct {
		Me
		LastSeq *int64 `json:"last_seq"`
	}
	if _, _, err := c.authed(ctx, gen, http.MethodGet, "/api/v1/me", nil, nil, &got); err != nil {
		return err
	}
	me := got.Me
	c.mu.Lock()
	if c.gen != gen {
		c.mu.Unlock()
		return ErrNotLinked
	}
	if c.unpark(me.Learner.ID) {
		// Events kept when this learner's link was lost: written to
		// the queue before they leave the parked file, so a crash
		// between the two never loses them; the next start keeps one
		// copy of each (dropQueued, mergeQueues).
		c.mu.Unlock()
		if err := c.saveQueue(); err != nil {
			return err
		}
		c.mu.Lock()
		if err := c.writeParked(); err != nil {
			// The events are in the queue file and still in the parked
			// file: the next start keeps one copy (dropQueued).
			c.mu.Unlock()
			return err
		}
		if c.gen != gen {
			c.mu.Unlock()
			return ErrNotLinked
		}
	}
	defer c.mu.Unlock()
	c.aiOff = false // the learner, as the server says now
	save := false
	if got.LastSeq != nil && saneSeq(*got.LastSeq) && *got.LastSeq+1 > c.st.NextSeq {
		// The saved number was too low (a game that quit without
		// saving it): new events would be taken as duplicates.
		c.st.NextSeq = *got.LastSeq + 1
		save = true
	}
	old, _ := json.Marshal(c.st.Me)
	now, _ := json.Marshal(&me)
	if string(old) != string(now) {
		c.st.Me = &me
		c.changes++
		save = true
	}
	if save {
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
		// The sent mark of totals is on disk before the request: a game
		// that dies after the server stored the batch must not change
		// those events when it starts again. If it can't be written, the
		// send goes on (and Close tries again).
		c.saveQueue()

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
			if saneSeq(res.LastSeq) {
				c.st.NextSeq = max(c.st.NextSeq, res.LastSeq+1)
			}
			c.batch = min(MaxBatch, c.batch*2)
			// Saved at once, so a game that quits now, or a sync that
			// fails later, never hands out these numbers again. If it
			// fails, me's last_seq puts it right at the next sync.
			c.saveState()
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
			ctx, cancel := context.WithTimeout(c.life, syncTimeout)
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
			ctx, cancel := context.WithTimeout(c.life, syncTimeout)
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

// syncStopWait is how long Close waits for a sync it has stopped.
const syncStopWait = 2 * time.Second

// Close stops the background sync, tries for a moment to send what is
// queued, and saves the queue. The game calls it when it quits.
func (c *Client) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	stop, stopped := c.stop, c.stopped
	c.stop = nil
	play := c.play
	linked, pending := c.st.linked(), c.q.len()
	c.mu.Unlock()
	if stop != nil {
		close(stop)
	}
	play.Leave()
	// Stop a sign-in or sync still on its way, and give it a moment to
	// end: it writes the learner's folder, which the game may move or
	// remove as soon as Close returns.
	c.mu.Lock()
	c.endLife() // under c.mu: no audio run starts (bg.Add) after it
	c.mu.Unlock()
	gotSync := false
	for deadline := time.Now().Add(syncStopWait); ; time.Sleep(5 * time.Millisecond) {
		if gotSync = c.syncMu.TryLock(); gotSync || time.Now().After(deadline) {
			break
		}
	}
	if gotSync {
		if linked && pending > 0 {
			ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
			c.mu.Lock()
			gen := c.gen
			c.mu.Unlock()
			c.upload(ctx, gen)
			cancel()
		}
		c.syncMu.Unlock()
	}
	c.save(false)
	c.mu.Lock()
	if c.stateUnsaved && c.saveState() == nil {
		c.stateUnsaved = false
	}
	c.mu.Unlock()
	// An unlink the server hasn't heard about yet gets a moment too.
	waited := make(chan struct{})
	go func() { c.bg.Wait(); close(waited) }()
	select {
	case <-waited:
	case <-time.After(closeTimeout):
	}
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
// network; a sync still running throws away what it gets. In the
// background it tells the server (POST /api/v1/unlink), so the game
// leaves the learner's page on the website; if that fails (offline), the
// family can still remove it there.
func (c *Client) Unlink() {
	if c == nil {
		return
	}
	c.mu.Lock()
	access, refresh, exp := c.st.Access, c.st.Refresh, c.st.AccessExp
	linked := c.st.linked()
	c.unlink(false)
	play := c.play
	c.mu.Unlock()
	play.Leave()
	if !linked {
		return
	}
	c.bg.Add(1)
	go func() {
		defer c.bg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), unlinkTimeout)
		defer cancel()
		c.tellUnlinked(ctx, access, refresh, exp)
	}()
}

// unlinkTimeout is how long Unlink keeps trying to tell the server.
const unlinkTimeout = 20 * time.Second

// tellUnlinked asks the server to unlink the device whose tokens these
// were. An access token that has run out is first swapped with the
// refresh token (the new pair is only used for this). A 401 means the
// server has unlinked it already.
func (c *Client) tellUnlinked(ctx context.Context, access, refresh string, exp time.Time) error {
	if access == "" || c.now().After(exp) {
		if refresh == "" {
			return ErrNotLinked
		}
		payload, _ := json.Marshal(map[string]string{"refresh_token": refresh})
		var t tokens
		if _, _, err := c.once(ctx, http.MethodPost, "/api/v1/token", "", nil, payload, &t); err != nil {
			return err
		}
		access = t.AccessToken
	}
	_, _, err := c.once(ctx, http.MethodPost, "/api/v1/unlink", access, nil, nil, nil)
	return err
}

// revoke tells the server, in the background and best effort, to unlink
// the device whose tokens t are, for tokens the game got but won't keep
// (a sign-in the player gave up on while the answer was on its way). It
// uses the existing POST /api/v1/unlink, as Unlink does.
func (c *Client) revoke(t tokens) {
	exp := c.now().Add(time.Duration(t.ExpiresIn) * time.Second)
	c.bg.Add(1)
	go func() {
		defer c.bg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), unlinkTimeout)
		defer cancel()
		c.tellUnlinked(ctx, t.AccessToken, t.RefreshToken, exp)
	}()
}

// lost unlinks the game because the server no longer takes its tokens.
// The events not sent are kept for the learner (park), not forgotten.
func (c *Client) lost(gen int) {
	c.mu.Lock()
	if c.gen != gen || !c.st.linked() {
		c.mu.Unlock()
		return
	}
	// The events not sent wait for this learner to link again; the
	// server may only have lost track of a refresh (a lost answer).
	// They must not be lost when they can't be written now (full local
	// storage): see parkForUnlink.
	c.unlink(c.parkForUnlink())
	c.note = "This game was unlinked. Your progress is still here."
	play := c.play
	c.mu.Unlock()
	// As Unlink does: a game that isn't linked has no room to be in.
	play.Leave()
}

// unlink does Unlink. c.mu is held. With keepFiles, the queue file and the
// state file are left as they are on disk (parkForUnlink says when).
func (c *Client) unlink(keepFiles bool) {
	if c.o.Store != nil {
		for _, li := range c.st.Lists {
			c.moveOut(li)
		}
	}
	c.gen++
	if c.audioCancel != nil {
		c.audioCancel() // the pack downloads of the old link
	}
	c.st = state{NextSeq: c.st.NextSeq}
	c.q = queue{}
	c.dirty = false
	c.unlinkPending = keepFiles
	if c.o.Store != nil && !keepFiles {
		c.o.Store.Remove(queueFile)
	}
	c.memories, c.packFails = map[string]*fetched{}, nil
	c.err, c.failures, c.note = nil, 0, ""
	c.loadLists()
	if !keepFiles {
		c.saveState()
	}
	c.changes++
}
