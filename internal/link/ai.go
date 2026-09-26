package link

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"time"
)

// aiTimeout is how long a request to Halpwords AI may take: the server
// asks a model, which can take a while.
const aiTimeout = 75 * time.Second

// The error codes of POST /api/v1/ai/{task} (docs/api/ai.md on the
// server).
const (
	CodeAIOff      = "ai_off"       // the account's plan doesn't include it, or it is turned off
	CodeAIBudget   = "ai_budget"    // the month's allowance is used up
	CodeAINoOutput = "ai_no_output" // nothing the server could use: don't ask again at once
	CodeAIBusy     = "ai_busy"      // the model is busy: wait a while
	CodeNoWords    = "no_words"     // none of the words are the learner's
)

// ErrNoAI is a request to Halpwords AI from a game that can't use it:
// not linked, or the server doesn't offer the task.
var ErrNoAI = errors.New("Halpwords AI isn't available to this game")

// AIAvailable reports whether the game can use Halpwords AI: it is linked
// and the server last said the account's plan includes it. It doesn't
// wait for the network.
func (c *Client) AIAvailable() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.st.linked() && !c.aiOff && c.st.Me != nil && c.st.Me.AI != nil && c.st.Me.AI.Available
}

// AskAI sends req to Halpwords AI's task (POST /api/v1/ai/{task}) and
// reads the answer into out. It isn't tried again after a failure: the
// game's AI decides when to ask again. Errors from the server are
// *Error, with one of the Code constants above.
func (c *Client) AskAI(ctx context.Context, task string, req, out any) error {
	if c == nil {
		return ErrNoAI
	}
	c.mu.Lock()
	gen := c.gen
	ok := c.st.linked() && c.st.Me != nil && c.st.Me.AI != nil && slices.Contains(c.st.Me.AI.Tasks, task)
	c.mu.Unlock()
	if !ok {
		return ErrNoAI
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}
	token, err := c.aiToken(ctx, gen, "")
	if err != nil {
		return err
	}
	hc := *c.hc
	hc.Timeout = aiTimeout
	path := "/api/v1/ai/" + url.PathEscape(task)
	_, _, err = c.onceWith(ctx, &hc, aiTimeout, http.MethodPost, path, token, nil, payload, out)
	var e *Error
	if errors.As(err, &e) && e.Status == http.StatusUnauthorized {
		if token, err = c.aiToken(ctx, gen, token); err != nil {
			return err
		}
		_, _, err = c.onceWith(ctx, &hc, aiTimeout, http.MethodPost, path, token, nil, payload, out)
	}
	if errors.As(err, &e) && e.Code == CodeAIOff {
		c.mu.Lock()
		c.aiOff = true
		c.mu.Unlock()
	}
	return err
}

// aiToken returns an access token that is good for a while, refreshing it
// first when it is about to run out or is refused (the one the server
// refused). It holds syncMu only while it does, so a slow answer from
// Halpwords AI never holds up a sync.
func (c *Client) aiToken(ctx context.Context, gen int, refused string) (string, error) {
	for !c.syncMu.TryLock() {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	defer c.syncMu.Unlock()
	c.mu.Lock()
	if c.gen != gen || !c.st.linked() {
		c.mu.Unlock()
		return "", ErrNotLinked
	}
	token, exp := c.st.Access, c.st.AccessExp
	c.mu.Unlock()
	if token == "" || token == refused || c.now().Add(refreshEarly).After(exp) {
		if err := c.refresh(ctx, gen); err != nil {
			return "", err
		}
		token = c.AccessToken()
	}
	return token, nil
}
