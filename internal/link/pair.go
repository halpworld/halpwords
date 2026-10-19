package link

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Linking by QR code (halpwords-server#153): the game asks the server for
// a code, shows it as a QR code and as ABCD-EFGH, and a grown-up scans it
// (or types the code on the website's /link page) and picks the child to
// link. The game polls until they have. It is the same device flow as
// signing in with a school account (sso.go), with a PKCE verifier, but a
// guardian approves it, and the tokens are kept as after a typed pairing
// code (WayPairing).

// Errors of linking by QR code, for the Account screen.
var (
	// ErrPairPending: no grown-up has approved the code yet.
	ErrPairPending = errors.New("waiting for a grown-up to scan the code")
	// ErrPairExpired: the code ran out or was used: get a new one.
	ErrPairExpired = errors.New("that code has run out: get a new one")
	// ErrPairRefused: the grown-up said this isn't their game.
	ErrPairRefused = errors.New("the grown-up said this isn't their game")
	// ErrPairOff: the server can't link by QR code (an older server, or
	// it is turned off): the game links with a typed code instead.
	ErrPairOff = errors.New("this server can't link by QR code: type a code instead")
	// ErrUnconfirmed is a sync of a game linked by QR code before the
	// child said the learner is them (ConfirmPair).
	ErrUnconfirmed = errors.New("check who this game is linked to first")
)

// The pair flow's own error code: 404 link_off.
const codeLinkOff = "link_off"

// PairCode is a link by QR code on its way. The grown-up scans ScanURL
// (or opens URL and types UserCode), and the game polls with the device
// code, which it keeps in link.json so a web game that reloads goes on.
type PairCode struct {
	DeviceCode string    `json:"device_code"`
	UserCode   string    `json:"user_code"`
	URL        string    `json:"verification_uri"`
	VerifyURL  string    `json:"verification_uri_complete"`
	Expires    time.Time `json:"expires"`
	// Started is when the game asked for the code.
	Started time.Time `json:"started,omitempty"`
	// Every is how often the game may poll.
	Every time.Duration `json:"every"`
	// Verifier is the PKCE secret whose S256 challenge the start sent,
	// as for SSOCode. It stays in link.json (never exported, see
	// move.Exportable), is never logged or shown, and the copies the
	// game hands out leave it out.
	Verifier string `json:"code_verifier,omitempty"`
	// ScanURL is what the QR code holds: VerifyURL when it is an http or
	// https address on the server the game links to, else "" (the screen
	// then shows only the code and URL). Only in the copies handed out.
	ScanURL string `json:"-"`
}

// public is a copy of the code for the screens: without the verifier,
// with ScanURL.
func (p *PairCode) public(server string) *PairCode {
	cp := *p
	cp.Verifier = ""
	cp.ScanURL = ""
	if sameHost(p.VerifyURL, server) {
		cp.ScanURL = p.VerifyURL
	}
	return &cp
}

// sameHost reports whether addr is an http or https address on server's
// host: the QR code never sends a grown-up anywhere else.
func sameHost(addr, server string) bool {
	a, err := url.Parse(addr)
	if err != nil || a.Scheme != "https" && a.Scheme != "http" || a.User != nil || a.Host == "" {
		return false
	}
	s, err := url.Parse(server)
	if err != nil || s.Hostname() == "" {
		return false
	}
	return strings.EqualFold(a.Hostname(), s.Hostname())
}

// deviceStart is the server's answer to the start of a device flow
// (POST /api/v1/sso/start and /api/v1/link/start).
type deviceStart struct {
	DeviceCode string `json:"device_code"`
	UserCode   string `json:"user_code"`
	URL        string `json:"verification_uri"`
	VerifyURL  string `json:"verification_uri_complete"`
	ExpiresIn  int64  `json:"expires_in"`
	Interval   int64  `json:"interval"`
}

// ok reports whether the answer has a code the game can use.
func (d *deviceStart) ok() bool {
	return d.DeviceCode != "" && d.UserCode != "" && strings.HasPrefix(d.URL, "http")
}

// pollEvery is how often to poll for a server's interval in seconds,
// kept between minPoll and maxPoll.
func pollEvery(interval int64) time.Duration {
	return min(max(time.Duration(interval)*time.Second, minPoll), maxPoll)
}

// slower is the poll interval after a slow_down: 5 seconds more (RFC
// 8628 section 3.5), up to maxPoll.
func slower(every time.Duration) time.Duration {
	return min(every+5*time.Second, maxPoll)
}

// StartPair asks the server for a code to link the game by QR code. It
// returns ErrPairOff when the server can't (any 404 or 405: an older
// server has no such route), and then keeps saying so without asking
// until the game restarts. A start whose ctx is cancelled keeps nothing.
// It waits for the answer, so call it from a goroutine.
func (c *Client) StartPair(ctx context.Context) (*PairCode, error) {
	if c.Linked() {
		return nil, ErrLinked
	}
	c.mu.Lock()
	off := c.pairOff
	c.mu.Unlock()
	if off {
		return nil, ErrPairOff
	}
	verifier, challenge, err := newVerifier()
	if err != nil {
		return nil, err
	}
	var out deviceStart
	_, _, err = c.call(ctx, http.MethodPost, "/api/v1/link/start", "", nil,
		map[string]string{"code_challenge": challenge}, &out)
	var e *Error
	if errors.As(err, &e) && (e.Status == http.StatusNotFound || e.Status == http.StatusMethodNotAllowed || e.Code == codeLinkOff) {
		c.mu.Lock()
		c.pairOff = true
		c.mu.Unlock()
		return nil, ErrPairOff
	}
	if err = explainPair(err); err != nil {
		return nil, err
	}
	if !out.ok() {
		return nil, errors.New("link: the server sent no code")
	}
	now := c.now()
	code := &PairCode{DeviceCode: out.DeviceCode, UserCode: out.UserCode, URL: out.URL, VerifyURL: out.VerifyURL,
		Verifier: verifier, Started: now, Expires: now.Add(time.Duration(out.ExpiresIn) * time.Second),
		Every: pollEvery(out.Interval)}
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx.Err() != nil || c.st.linked() {
		// Cancelled (Esc) while the answer was on its way: nobody can
		// approve a code the game never showed.
		return nil, ErrPairExpired
	}
	c.st.Pair = code
	c.saveState()
	return code.public(c.server), nil
}

// PendingPair is the link by QR code on its way, or nil when there is
// none or it has run out.
func (c *Client) PendingPair() *PairCode {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.st.Pair == nil || c.st.linked() || !c.now().Before(c.st.Pair.Expires) {
		return nil
	}
	return c.st.Pair.public(c.server)
}

// CancelPair forgets the link by QR code on its way.
func (c *Client) CancelPair() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.st.Pair != nil {
		c.st.Pair = nil
		c.saveState()
	}
}

// PollPair asks once whether a grown-up has approved the code on its
// way. It returns nil once the game is linked (the first sync is the
// caller's, as after a typed code), ErrPairPending while nobody has, and
// when the link is over (the code is then forgotten) ErrPairExpired,
// ErrPairRefused, ErrNotLinkable or ErrSuspended. An answer that never
// came, a server error or a rate limit keeps the code until it runs out:
// the error is returned, and the caller polls again later. It waits for
// the answer, so call it from a goroutine, at most every PairCode.Every.
func (c *Client) PollPair(ctx context.Context) error {
	c.syncMu.Lock()
	defer c.syncMu.Unlock()
	c.mu.Lock()
	if c.st.linked() {
		c.mu.Unlock()
		return ErrLinked
	}
	code := c.st.Pair
	c.mu.Unlock()
	if code == nil || !c.now().Before(code.Expires) {
		c.dropPair(code)
		return ErrPairExpired
	}
	payload, _ := json.Marshal(map[string]string{
		"device_code": code.DeviceCode, "code_verifier": code.Verifier, "name": c.deviceName(),
	})
	var t tokens
	_, _, err := c.once(ctx, http.MethodPost, "/api/v1/link/token", "", nil, payload, &t)
	var apiErr *Error
	if errors.As(err, &apiErr) && apiErr.Code == codeSlowDown {
		c.slowPair(code)
	}
	again := err != nil && (!errors.As(err, &apiErr) || retryableStatus(apiErr))
	switch err = explainPair(err); {
	case errors.Is(err, ErrPairPending):
		return err
	case err == nil && (t.AccessToken == "" || t.RefreshToken == ""):
		err = errors.New("link: the server sent no tokens")
	case err == nil:
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.st.Pair != code || c.st.linked() {
			// Cancelled (Esc) or replaced while the answer was on its
			// way: the tokens are dropped, and the server told, so the
			// device isn't left linked with nobody holding it.
			c.revoke(t)
			return ErrPairExpired
		}
		// Nothing syncs until the child says the learner is them: a
		// grown-up who picked the wrong child (or scanned another
		// family's screen) must not get this game's progress. The hold
		// is in the first link.json written.
		c.signedInHeld(t, WayPairing, true)
		return nil
	}
	if again && c.now().Before(code.Expires) {
		return err
	}
	c.dropPair(code)
	if again {
		return ErrPairExpired // it ran out while the connection was bad
	}
	return err
}

// slowPair takes slow_down, and keeps it across a reload.
func (c *Client) slowPair(code *PairCode) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.st.Pair == code {
		code.Every = slower(code.Every)
		c.saveState()
	}
}

// dropPair forgets code, if it is still the link on its way: a poll that
// comes back late must not wipe a newer code.
func (c *Client) dropPair(code *PairCode) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if code != nil && c.st.Pair == code {
		c.st.Pair = nil
		c.saveState()
	}
}

// explainPair turns the server's error answer to a link by QR code into
// the error the screens explain.
func explainPair(err error) error {
	var e *Error
	if !errors.As(err, &e) {
		return err
	}
	switch e.Code {
	case codeAuthorizationPending, codeSlowDown:
		return ErrPairPending
	case codeExpiredToken:
		return ErrPairExpired
	case codeAccessDenied:
		return ErrPairRefused
	case codeNotLinkable:
		return ErrNotLinkable
	case codeAccountSuspended:
		return ErrSuspended
	case codeLinkOff:
		return ErrPairOff
	case codeRateLimited:
		return &busyError{e}
	}
	if e.Status == http.StatusTooManyRequests {
		return &busyError{e}
	}
	return err
}

// PairedLearner asks the server who the game linked by QR code is
// linked to (GET /api/v1/me), for the child to confirm before anything
// syncs. It sends nothing of the game's and keeps nothing: the first
// sync reads the learner again. It waits for the answer, so call it
// from a goroutine.
func (c *Client) PairedLearner(ctx context.Context) (string, error) {
	c.syncMu.Lock()
	defer c.syncMu.Unlock()
	c.mu.Lock()
	gen, linked := c.gen, c.st.linked()
	c.mu.Unlock()
	if !linked {
		return "", ErrNotLinked
	}
	var me Me
	if _, _, err := c.authed(ctx, gen, http.MethodGet, "/api/v1/me", nil, nil, &me); err != nil {
		return "", err
	}
	return me.Learner.DisplayName, nil
}

// ConfirmPair is the child saying the learner the game was linked to by
// QR code is them, once they have seen the child-link notice if one is
// due: syncing starts. It reports whether the game was waiting for it,
// so the caller syncs. Saying no is Unlink, which keeps everything on
// this computer and sends nothing.
func (c *Client) ConfirmPair() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.st.Unconfirmed {
		return false
	}
	c.st.Unconfirmed = false
	c.stateUnsaved = c.saveState() != nil
	return true
}
