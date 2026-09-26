package link

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Error is an error answer from the server (docs/api/error.schema.json).
type Error struct {
	Status  int
	Code    string
	Message string
	// RetryAfter is how long a rate_limited answer says to wait.
	RetryAfter time.Duration
	// Details are, for invalid_request, the paths of the problems.
	Details []struct {
		Path    string `json:"path"`
		Message string `json:"message"`
	}
}

func (e *Error) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s (%d %s)", e.Message, e.Status, e.Code)
	}
	return fmt.Sprintf("the server answered %d %s", e.Status, e.Code)
}

// The error codes the link acts on.
const (
	codeUnauthenticated = "unauthenticated"
	codeInvalidToken    = "invalid_token"
	codeTokenReused     = "token_reused"
	codeInvalidCode     = "invalid_code"
	codeNotLinkable     = "not_linkable"
	codeLocked          = "locked"
	codeInvalidRequest  = "invalid_request"
	codeTooLarge        = "too_large"
	codeRateLimited     = "rate_limited"
)

// Errors the Account screen explains.
var (
	// ErrBadCode is a pairing code that is wrong, used or too old.
	ErrBadCode = errors.New("that code didn't work: codes work once, for 15 minutes")
	// ErrNotLinkable is a learner who can't be linked now.
	ErrNotLinkable = errors.New("this learner can't be linked right now: ask your grown-up to check the website")
	// ErrUnlinked is a game the website unlinked, or whose tokens are
	// no longer good: it has to be linked again.
	ErrUnlinked = errors.New("this game was unlinked on the website")
	// ErrNotLinked is a sync of a game that isn't linked.
	ErrNotLinked = errors.New("this game isn't linked")
	// ErrLinked is a link of a game that already is.
	ErrLinked = errors.New("this game is already linked: unlink it first")
)

// Explain says what went wrong in a few words, for the Account screen.
func Explain(err error) string {
	var e *Error
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrBadCode), errors.Is(err, ErrNotLinkable), errors.Is(err, ErrUnlinked),
		errors.Is(err, ErrNotLinked), errors.Is(err, ErrLinked), errors.Is(err, ErrWrongSignIn), errors.Is(err, ErrLocked):
		return err.Error()
	case errors.Is(err, context.DeadlineExceeded):
		return "the server took too long to answer"
	case errors.As(err, &e) && e.Code == codeRateLimited:
		return "the server is busy; the game will try again later"
	case errors.As(err, &e) && e.Status >= 500:
		return "the server has a problem; the game will try again later"
	case errors.As(err, &e):
		return e.Error()
	}
	return "can't reach the server; the game will try again later"
}

// retryable reports whether a failed request may work if tried again
// soon: no answer at all, a server error, or a rate limit with a short
// wait.
func retryable(err error) (bool, time.Duration) {
	var e *Error
	if !errors.As(err, &e) {
		return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded), 0
	}
	switch {
	case e.Code == codeRateLimited || e.Status == http.StatusTooManyRequests:
		return e.RetryAfter <= 30*time.Second, e.RetryAfter
	case e.Status >= 500:
		return true, 0
	}
	return false, 0
}

// tries is how many times a request is made before its sync gives up.
const tries = 3

// retryWait is the wait before try n (from 1): 1, then 3 seconds.
func retryWait(n int) time.Duration { return time.Duration(2*n-1) * time.Second }

// call makes a request, trying again after a failure that may pass. body
// is sent as JSON when not nil; out, when not nil, is filled from a 200
// answer. It returns the answer's status and headers.
func (c *Client) call(ctx context.Context, method, path, token string, hdr http.Header, body, out any) (int, http.Header, error) {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return 0, nil, err
		}
	}
	for n := 1; ; n++ {
		status, h, err := c.once(ctx, method, path, token, hdr, payload, out)
		if err == nil {
			return status, h, nil
		}
		again, wait := retryable(err)
		if !again || n >= tries || ctx.Err() != nil {
			return status, h, err
		}
		c.sleep(max(wait, retryWait(n)))
	}
}

// once makes one request.
func (c *Client) once(ctx context.Context, method, path, token string, hdr http.Header, payload []byte, out any) (int, http.Header, error) {
	rctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(rctx, method, c.server+path, body)
	if err != nil {
		return 0, nil, err
	}
	for k, v := range hdr {
		req.Header[k] = v
	}
	if inBrowser {
		// A browser won't let a page set User-Agent (and one that does
		// makes every request need a CORS preflight for it).
		req.Header.Set(clientHeader, c.clientVersion())
	} else {
		req.Header.Set("User-Agent", c.userAgent())
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return resp.StatusCode, resp.Header, err
	}
	switch {
	case resp.StatusCode == http.StatusNotModified:
		return resp.StatusCode, resp.Header, nil
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		if out != nil {
			if err := json.Unmarshal(data, out); err != nil {
				return resp.StatusCode, resp.Header, fmt.Errorf("link: %s %s: %w", method, path, err)
			}
		}
		return resp.StatusCode, resp.Header, nil
	}
	return resp.StatusCode, resp.Header, errorFrom(resp.StatusCode, resp.Header, data)
}

// errorFrom is the *Error of an error answer: its status, the code and
// message of its JSON body, and Retry-After.
func errorFrom(status int, h http.Header, data []byte) *Error {
	e := &Error{Status: status, Code: http.StatusText(status)}
	var eb struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details []struct {
				Path    string `json:"path"`
				Message string `json:"message"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &eb) == nil && eb.Error.Code != "" {
		e.Code, e.Message, e.Details = eb.Error.Code, eb.Error.Message, eb.Error.Details
	}
	if s, err := strconv.Atoi(h.Get("Retry-After")); err == nil && s >= 0 {
		e.RetryAfter = time.Duration(s) * time.Second
	}
	return e
}

// tokens is the answer to POST /api/v1/link and /api/v1/token.
type tokens struct {
	DeviceID         string `json:"device_id"`
	TokenType        string `json:"token_type"`
	AccessToken      string `json:"access_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshToken     string `json:"refresh_token"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
}

// keep stores a new pair of tokens, on disk before they are used: the old
// refresh token no longer works. c.mu is held.
func (c *Client) keep(t tokens) {
	now := c.now()
	c.st.Server = c.server
	if t.DeviceID != "" {
		c.st.DeviceID = t.DeviceID
	}
	c.st.Access, c.st.AccessExp = t.AccessToken, now.Add(time.Duration(t.ExpiresIn)*time.Second)
	c.st.Refresh, c.st.RefreshExp = t.RefreshToken, now.Add(time.Duration(t.RefreshExpiresIn)*time.Second)
	c.saveState()
}

// refresh swaps the refresh token for new tokens. A refresh token the
// server no longer takes means the game was unlinked on the website: the
// game unlinks itself too. syncMu is held.
func (c *Client) refresh(ctx context.Context, gen int) error {
	c.mu.Lock()
	rt := c.st.Refresh
	c.mu.Unlock()
	if rt == "" {
		return ErrNotLinked
	}
	var t tokens
	// Never tried twice: a refresh token works once, and one the server
	// has seen before unlinks the game.
	payload, _ := json.Marshal(map[string]string{"refresh_token": rt})
	_, _, err := c.once(ctx, http.MethodPost, "/api/v1/token", "", nil, payload, &t)
	var e *Error
	if errors.As(err, &e) && e.Status == http.StatusUnauthorized {
		// invalid_token, token_reused, or anything else a 401 says: these
		// tokens are finished.
		c.lost(gen)
		return ErrUnlinked
	}
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gen != gen {
		return ErrNotLinked
	}
	c.keep(t)
	return nil
}

// authed makes a request as the device, refreshing the access token when
// it is about to run out or the server says it has.
func (c *Client) authed(ctx context.Context, gen int, method, path string, hdr http.Header, body, out any) (int, http.Header, error) {
	c.mu.Lock()
	if c.gen != gen || !c.st.linked() {
		c.mu.Unlock()
		return 0, nil, ErrNotLinked
	}
	token, exp := c.st.Access, c.st.AccessExp
	c.mu.Unlock()
	if token == "" || c.now().Add(refreshEarly).After(exp) {
		if err := c.refresh(ctx, gen); err != nil {
			return 0, nil, err
		}
		token = c.AccessToken()
	}
	status, h, err := c.call(ctx, method, path, token, hdr, body, out)
	var e *Error
	if errors.As(err, &e) && e.Status == http.StatusUnauthorized {
		if err := c.refresh(ctx, gen); err != nil {
			return status, h, err
		}
		return c.call(ctx, method, path, c.AccessToken(), hdr, body, out)
	}
	return status, h, err
}
