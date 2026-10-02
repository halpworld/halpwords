package link

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// WaySSO is a sign-in with a school Google or Microsoft account (W5.4,
// the server's docs/api "Signing in with a school account").
const WaySSO = "sso"

// Errors of signing in with a school account, for the sign-in screen.
var (
	// ErrSSOPending: the pupil hasn't signed in on the website yet.
	ErrSSOPending = errors.New("waiting for you to sign in on the website")
	// ErrSSOExpired: the code ran out or was used: start again.
	ErrSSOExpired = errors.New("that code has run out: start again")
	// ErrSSORefused: the account can't sign in as a pupil here.
	ErrSSORefused = errors.New("that account can't sign in here: ask your teacher")
	// ErrSSOOff: the server doesn't offer school accounts.
	ErrSSOOff = errors.New("school accounts can't be used with this server")
	// ErrSSOBusy: too many tries in a short time. The error explainSSO
	// returns for it also unwraps to the *Error, with its RetryAfter.
	ErrSSOBusy = errors.New("too many tries: wait a moment and try again")
)

// busyError is a rate_limited answer to the sign-in, for the screens.
type busyError struct{ e *Error }

func (b *busyError) Error() string {
	if s := int((b.e.RetryAfter + time.Second - 1) / time.Second); s > 0 && s < 60 {
		return fmt.Sprintf("too many tries: wait %d seconds and try again", s)
	}
	return ErrSSOBusy.Error()
}

func (b *busyError) Unwrap() []error { return []error{ErrSSOBusy, b.e} }

// The device flow's error codes (RFC 8628).
const (
	codeAuthorizationPending = "authorization_pending"
	codeSlowDown             = "slow_down"
	codeExpiredToken         = "expired_token"
	codeAccessDenied         = "access_denied"
	codeSSOOff               = "sso_off"
)

// SSOCode is a sign-in with a school account on its way: the pupil opens
// URL (or VerifyURL, which needs no typing) and types UserCode, and the
// game polls with the device code, which it keeps in link.json so a web
// game that left the page for the website can go on when it comes back.
type SSOCode struct {
	DeviceCode string    `json:"device_code"`
	UserCode   string    `json:"user_code"`
	URL        string    `json:"verification_uri"`
	VerifyURL  string    `json:"verification_uri_complete"`
	Expires    time.Time `json:"expires"`
	// Started is when the game asked for the code.
	Started time.Time `json:"started,omitempty"`
	// Every is how often the game may poll.
	Every time.Duration `json:"every"`
	// Verifier is the secret whose S256 challenge the game sent with the
	// start (PKCE, RFC 7636): the server gives the tokens only to a poll
	// that sends it, so a copy of DeviceCode alone is no use. It stays
	// in link.json (never exported, see move.Exportable), is never
	// logged, and the copies of the code the game hands out leave it
	// out.
	Verifier string `json:"code_verifier,omitempty"`
}

// public is a copy of the code for the screens, without the verifier.
func (c *SSOCode) public() *SSOCode {
	cp := *c
	cp.Verifier = ""
	return &cp
}

// newVerifier makes a PKCE code verifier: 32 random bytes, unpadded
// base64url (43 characters), and its S256 challenge.
func newVerifier() (verifier, challenge string, err error) {
	var b [32]byte
	if _, err = rand.Read(b[:]); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b[:])
	sum := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

// minPoll is the least time between polls, whatever the server says.
const minPoll = 2 * time.Second

// maxPoll is the most time between polls, whatever the server says and
// however often it says slow_down.
const maxPoll = time.Minute

// StartSSO asks the server for a code to sign in with a school account.
// returnTo is where the web game wants the website to send the pupil
// back to, or "". It binds the code to this game with a PKCE verifier. A
// server from before that (halpwords-server#98) ignores the challenge
// and the verifier, and the code works as it did. A start whose ctx is
// cancelled keeps nothing. It waits for the answer, so call it from a
// goroutine.
func (c *Client) StartSSO(ctx context.Context, returnTo string) (*SSOCode, error) {
	if c.Linked() {
		return nil, ErrLinked
	}
	verifier, challenge, err := newVerifier()
	if err != nil {
		return nil, err
	}
	body := map[string]string{"code_challenge": challenge}
	if returnTo != "" {
		body["return_to"] = returnTo
	}
	var out struct {
		DeviceCode string `json:"device_code"`
		UserCode   string `json:"user_code"`
		URL        string `json:"verification_uri"`
		VerifyURL  string `json:"verification_uri_complete"`
		ExpiresIn  int64  `json:"expires_in"`
		Interval   int64  `json:"interval"`
	}
	_, _, err = c.call(ctx, http.MethodPost, "/api/v1/sso/start", "", nil, body, &out)
	if err = explainSSO(err); err != nil {
		return nil, err
	}
	if out.DeviceCode == "" || out.UserCode == "" || !strings.HasPrefix(out.URL, "http") {
		return nil, errors.New("link: the server sent no code")
	}
	code := &SSOCode{DeviceCode: out.DeviceCode, UserCode: out.UserCode, URL: out.URL, VerifyURL: out.VerifyURL,
		Verifier: verifier, Started: c.now(),
		Expires: c.now().Add(time.Duration(out.ExpiresIn) * time.Second),
		Every:   min(max(time.Duration(out.Interval)*time.Second, minPoll), maxPoll)}
	if !strings.HasPrefix(code.VerifyURL, "http") {
		code.VerifyURL = code.URL
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx.Err() != nil || c.st.linked() {
		// Cancelled (Esc) while the answer was on its way: the pupil
		// chose to stop, and nothing was signed in with the code.
		return nil, ErrSSOExpired
	}
	c.st.SSO = code
	c.saveState()
	return code.public(), nil
}

// PendingSSO is the sign-in with a school account on its way, or nil
// when there is none or it has run out.
func (c *Client) PendingSSO() *SSOCode {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.st.SSO == nil || c.st.linked() || !c.now().Before(c.st.SSO.Expires) {
		return nil
	}
	return c.st.SSO.public()
}

// CancelSSO forgets the sign-in with a school account on its way.
func (c *Client) CancelSSO() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.st.SSO != nil {
		c.st.SSO = nil
		c.saveState()
	}
}

// PollSSO asks once whether the pupil has signed in with the code on its
// way. It returns nil once the game is linked, ErrSSOPending while the
// pupil hasn't signed in, and ErrSSOExpired, ErrSSORefused or another
// error when the sign-in is over (the code is then forgotten). It waits
// for the answer, so call it from a goroutine, at most every
// SSOCode.Every.
func (c *Client) PollSSO(ctx context.Context) error {
	c.syncMu.Lock()
	defer c.syncMu.Unlock()
	c.mu.Lock()
	if c.st.linked() {
		c.mu.Unlock()
		return ErrLinked
	}
	code := c.st.SSO
	c.mu.Unlock()
	if code == nil || !c.now().Before(code.Expires) {
		c.dropSSO(code)
		return ErrSSOExpired
	}
	req := map[string]string{"device_code": code.DeviceCode, "name": c.deviceName()}
	if code.Verifier != "" {
		req["code_verifier"] = code.Verifier
	}
	payload, _ := json.Marshal(req)
	var t tokens
	_, _, err := c.once(ctx, http.MethodPost, "/api/v1/sso/token", "", nil, payload, &t)
	var apiErr *Error
	if errors.As(err, &apiErr) && apiErr.Code == codeSlowDown {
		c.slowSSO(code)
	}
	// An answer that never came (a request that timed out, a dropped
	// connection) or a server error says nothing about the code: it may
	// still work, so keep it until it runs out.
	again := err != nil && (!errors.As(err, &apiErr) || retryableStatus(apiErr))
	switch err = explainSSO(err); {
	case errors.Is(err, ErrSSOPending):
		return err
	case err == nil && (t.AccessToken == "" || t.RefreshToken == ""):
		err = errors.New("link: the server sent no tokens")
	case err == nil:
		c.mu.Lock()
		if c.st.SSO != code || c.st.linked() {
			c.mu.Unlock()
			// Cancelled (Esc) or replaced while the answer was on its
			// way: the pupil chose to stop, so the tokens are dropped,
			// and the server told, so the device isn't left signed in
			// with nobody holding it.
			c.revoke(t)
			return ErrSSOExpired
		}
		defer c.mu.Unlock()
		c.signedIn(t, WaySSO)
		return nil
	}
	if again && c.now().Before(code.Expires) || errors.Is(err, ErrNotLinkable) {
		// The code may still work: try again later. A pupil who can't
		// be linked yet (no consent, say) may be by the time it runs
		// out, once a grown-up sees to it on the website.
		return err
	}
	c.dropSSO(code)
	if again {
		return ErrSSOExpired // it ran out while the connection was bad
	}
	return err
}

// slowSSO takes slow_down: the game polls 5 seconds less often from now
// on (RFC 8628 section 3.5), and keeps that across a reload.
func (c *Client) slowSSO(code *SSOCode) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.st.SSO == code {
		code.Every = min(code.Every+5*time.Second, maxPoll)
		c.saveState()
	}
}

// dropSSO forgets code, if it is still the sign-in on its way: a poll
// that comes back late must not wipe a newer code.
func (c *Client) dropSSO(code *SSOCode) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if code != nil && c.st.SSO == code {
		c.st.SSO = nil
		c.saveState()
	}
}

// retryableStatus reports whether the server's error answer is one that
// passes: a server error or a rate limit, however long it says to wait
// (the screen waits that long, up to a limit, before it asks again).
func retryableStatus(e *Error) bool {
	return e.Code == codeRateLimited || e.Status == http.StatusTooManyRequests || e.Status >= 500
}

// explainSSO turns the server's error answer to a sign-in with a school
// account into the error the screens explain.
func explainSSO(err error) error {
	var e *Error
	if !errors.As(err, &e) {
		return err
	}
	switch e.Code {
	case codeAuthorizationPending, codeSlowDown:
		return ErrSSOPending
	case codeExpiredToken:
		return ErrSSOExpired
	case codeAccessDenied:
		return ErrSSORefused
	case codeSSOOff:
		return ErrSSOOff
	case codeNotLinkable:
		return ErrNotLinkable
	case codeRateLimited:
		return &busyError{e}
	}
	if e.Status == http.StatusTooManyRequests {
		return &busyError{e}
	}
	return err
}
