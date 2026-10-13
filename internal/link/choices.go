package link

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// My choices (halpwords-server PLAN §25, 2026-10-12; consent rule R6):
// what the linked learner can see and choose about their own data on the
// server. Every function here waits for the server, so call it from a
// goroutine. They work while the learner's progress is stopped: these
// routes, and unlinking, are the ones the server still answers then.

// Notice is a versioned notice from the server, shown as it is. Ref
// names its version; the game sends it back to say it showed it.
type Notice struct {
	Ref   string   `json:"ref"`
	Title string   `json:"title"`
	Body  []string `json:"body"`
}

// Ask is something the learner is asked to agree to, such as being in
// friend groups or on ranking boards.
type Ask struct {
	Purpose string `json:"purpose"`
	Agreed  bool   `json:"agreed"`
	Label   string `json:"label"`
	Hint    string `json:"hint"`
}

// Visibility is the "who can see your progress" notice for teenagers and
// what they are asked to agree to under it.
type Visibility struct {
	Notice
	Asks []Ask `json:"asks"`
}

// Restart is the notice and button for asking to keep progress again.
type Restart struct {
	Notice
	Label string `json:"label"`
	Hint  string `json:"hint"`
}

// OwnProgress is where the learner's own progress stands: whether they
// can stop it being kept (a family's child of 16 or 17), whether they
// have, and whether a grown-up has still to confirm keeping it again.
type OwnProgress struct {
	CanStop bool     `json:"can_stop"`
	Stopped bool     `json:"stopped"`
	Waiting bool     `json:"waiting"`
	Restart *Restart `json:"restart,omitempty"`
	// Ask is a family's child under 16 asking their grown-up to stop
	// keeping their progress (servers from 2026-10-13); nil when it
	// doesn't apply, or from an older server.
	Ask *ProgressAsk `json:"ask,omitempty"`
}

// ProgressAsk is where a child under 16 stands with asking their
// grown-up to stop keeping their progress. The ask changes nothing by
// itself: only a grown-up can stop it, on the website.
type ProgressAsk struct {
	// CanAsk offers "Ask my grown-up to stop keeping my progress".
	CanAsk bool `json:"can_ask"`
	// WaitingSince is when they asked, while no grown-up has answered
	// (they can take it back); zero otherwise. The ask ends by itself
	// after 14 days.
	WaitingSince time.Time `json:"waiting_since,omitempty"`
	// Kept: a grown-up decided to keep their progress. Say only that,
	// and when they can ask again (AskAgainAfter).
	Kept bool `json:"kept"`
	// AskAgainAfter is when a kept answer ends and they can ask again;
	// zero otherwise.
	AskAgainAfter time.Time `json:"ask_again_after,omitempty"`
}

// AskDate is how the game writes an ask's dates: the UTC calendar date,
// as the website does, so both say the same day (halpwords-server PLAN
// §25 2026-10-13).
func AskDate(t time.Time) string { return t.UTC().Format("2 January 2006") }

// Choices is GET /api/v1/choices: only what applies to the learner is
// there.
type Choices struct {
	// Family is set for a family's learner; others only get ChildNotice.
	Family bool `json:"family"`
	// ChildNotice says who can see the learner's progress.
	ChildNotice Notice `json:"child_notice"`
	// OlderNotice is set when the one-time notice for 16- and
	// 17-year-olds is due. The server counts it as told by this answer
	// (it is set in one answer only), so show it from this Choices,
	// never from a later fetch.
	OlderNotice bool         `json:"older_notice"`
	Visibility  *Visibility  `json:"visibility,omitempty"`
	Progress    *OwnProgress `json:"progress,omitempty"`
}

// Errors of My choices.
var (
	// ErrNoChoices is a server without My choices (one older than
	// 2026-10-12; GET /api/v1/choices answers 404): the game shows
	// nothing more than before.
	ErrNoChoices = errors.New("my choices need a newer server")
	// ErrNoticeChanged is a notice that changed since it was fetched:
	// fetch Choices again and show the new words.
	ErrNoticeChanged = errors.New("the words changed: read them again")
	// ErrNotOwn is a choice the learner can't make themself now.
	ErrNotOwn = errors.New("you can't change that yourself now")
	// ErrAskedTooOften is a learner who asked to keep their progress
	// again, or to stop it, a few times today already (429
	// asked_too_often): each ask emails their grown-ups.
	ErrAskedTooOften = errors.New("you've asked a few times already: you can ask again later")
)

// The error codes of My choices.
const (
	codeNoticeChanged = "notice_changed"
	codeNotAsked      = "not_asked"
	codeNotOwn        = "not_own"
	codeAskedTooOften = "asked_too_often"
)

// own makes a My choices request as the device. It waits for a sync that
// is running, as both may refresh the tokens.
func (c *Client) own(ctx context.Context, method, path string, body, out any) error {
	if c == nil {
		return ErrNotLinked
	}
	for !c.syncMu.TryLock() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	defer c.syncMu.Unlock()
	c.mu.Lock()
	gen := c.gen
	c.mu.Unlock()
	_, _, err := c.authed(ctx, gen, method, path, nil, body, out)
	var e *Error
	switch {
	case errors.As(err, &e) && e.Status == http.StatusNotFound && method == http.MethodGet:
		// Only the GET tells an older server: every server answers a
		// route it doesn't have with 404 not_found, the same as a
		// newer server's own "not found", so the code can't tell them
		// apart. A choice is only offered after the GET worked, so a
		// 404 there is a normal error, shown as it is.
		return ErrNoChoices
	case errors.As(err, &e) && e.Code == codeNoticeChanged:
		return ErrNoticeChanged
	case errors.As(err, &e) && (e.Code == codeNotAsked || e.Code == codeNotOwn):
		return ErrNotOwn
	case errors.As(err, &e) && e.Code == codeAskedTooOften:
		return ErrAskedTooOften
	}
	return err
}

// Choices fetches My choices. It also notes whether the learner's
// progress is stopped, so the game stops or starts queuing at once.
func (c *Client) Choices(ctx context.Context) (*Choices, error) {
	var ch Choices
	if err := c.own(ctx, http.MethodGet, "/api/v1/choices", nil, &ch); err != nil {
		return nil, err
	}
	stopped := ch.Progress != nil && ch.Progress.Stopped
	c.mu.Lock()
	again := c.st.Stopped && !stopped
	switch {
	case stopped && c.st.linked():
		c.stopProgress()
	case again:
		// Kept again, as the server says: the game queues again at once
		// and syncs (the server drops anything from before, S1).
		c.st.Stopped = false
		c.changes++
		c.saveState()
	}
	c.mu.Unlock()
	if again {
		c.SyncNow()
	}
	return &ch, nil
}

// ChildNoticeDue reports whether the child-link notice has yet to be
// shown on this link (SawNotice).
func (c *Client) ChildNoticeDue() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.st.linked() && c.st.ChildNotice == ""
}

// SawNotice tells the server the child-link notice ref was shown, and
// remembers it on this link.
func (c *Client) SawNotice(ctx context.Context, ref string) error {
	if err := c.own(ctx, http.MethodPost, "/api/v1/choices/notice", map[string]string{"notice_ref": ref}, nil); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.st.linked() {
		c.st.ChildNotice = ref
		c.saveState()
	}
	return nil
}

// MarkOlder tells the server the one-time notice for 16- and
// 17-year-olds was shown. Servers from 2026-10-12 count it as told when
// GET /api/v1/choices sets OlderNotice and take this as a no-op; it is
// kept for servers that still wait for it.
func (c *Client) MarkOlder(ctx context.Context) error {
	return c.own(ctx, http.MethodPost, "/api/v1/choices/older", nil, nil)
}

// SetAgreement agrees to purpose, on the Visibility notice ref that was
// shown, or (agree false) withdraws from it.
func (c *Client) SetAgreement(ctx context.Context, purpose string, agree bool, ref string) error {
	body := map[string]any{"purpose": purpose, "agree": agree}
	if agree {
		body["notice_ref"] = ref
	}
	return c.own(ctx, http.MethodPost, "/api/v1/choices/agreement", body, nil)
}

// StopProgress stops the learner's progress being kept, once they have
// confirmed it: the server deletes it and tells their grown-ups. The game
// stays linked, drops what waits to be sent and queues nothing more.
func (c *Client) StopProgress(ctx context.Context) error {
	if err := c.own(ctx, http.MethodPost, "/api/v1/choices/progress/stop", map[string]bool{"confirm": true}, nil); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.st.linked() {
		c.stopProgress()
	}
	return nil
}

// StartProgress asks to keep the learner's progress again, on the
// Restart notice ref that was shown. A grown-up confirms it by email;
// until then the progress stays stopped (OwnProgress.Waiting).
func (c *Client) StartProgress(ctx context.Context, ref string) error {
	return c.own(ctx, http.MethodPost, "/api/v1/choices/progress/start", map[string]string{"notice_ref": ref}, nil)
}

// CancelProgress takes back a request to keep the progress again that
// no grown-up has confirmed yet.
func (c *Client) CancelProgress(ctx context.Context) error {
	return c.own(ctx, http.MethodPost, "/api/v1/choices/progress/cancel", nil, nil)
}

// AskStopProgress asks the learner's grown-ups to stop keeping their
// progress, once the learner said yes: every guardian gets an email, and
// nothing else changes (OwnProgress.Ask.WaitingSince). The game keeps
// syncing: only a grown-up can stop it. If an ask was already waiting
// (a stale screen), nobody is emailed and already is when it was asked;
// it is zero for a new ask.
func (c *Client) AskStopProgress(ctx context.Context) (already time.Time, err error) {
	var out struct {
		Already      bool      `json:"already"`
		WaitingSince time.Time `json:"waiting_since,omitempty"`
	}
	if err := c.own(ctx, http.MethodPost, "/api/v1/choices/progress/ask", map[string]bool{"confirm": true}, &out); err != nil {
		return time.Time{}, err
	}
	if out.Already {
		return out.WaitingSince, nil
	}
	return time.Time{}, nil
}

// TakeBackAsk takes back the learner's waiting ask to stop keeping their
// progress.
func (c *Client) TakeBackAsk(ctx context.Context) error {
	return c.own(ctx, http.MethodPost, "/api/v1/choices/progress/ask/cancel", nil, nil)
}
