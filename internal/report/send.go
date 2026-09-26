package report

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// DefaultServer is where reports go unless HALPWORDS_SERVER says
// otherwise.
const DefaultServer = "https://halpwords.com"

// ServerURL is the Halpwords server: HALPWORDS_SERVER if it is an https
// address (or http on this computer, for development), else
// DefaultServer. It has no trailing slash.
func ServerURL() string {
	s := strings.TrimRight(strings.TrimSpace(os.Getenv("HALPWORDS_SERVER")), "/")
	u, err := url.Parse(s)
	if s == "" || err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return DefaultServer
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return DefaultServer
	}
	return s
}

// Outcome is what became of a report sent.
type Outcome int

const (
	// Later: not sent now (offline, the server busy or down); keep it.
	Later Outcome = iota
	// Sent: the server has it (new, or from an earlier try).
	Sent
	// Refused: the server will never take it as it is; drop it.
	Refused
)

// Sender sends reports to the server.
type Sender struct {
	// Server is the server's address (ServerURL()).
	Server string
	// Token returns the linked game's access token, or "" when the game
	// isn't linked; nil is never linked. Without a token a report is
	// anonymous.
	Token func() string
	// UserAgent is the game's User-Agent, "Halpwords/1.2.0 (linux; amd64)".
	UserAgent string
	// Client sends the requests; nil is one with a 20-second timeout.
	Client *http.Client
}

var defaultClient = &http.Client{Timeout: 20 * time.Second}

// Send sends r once.
func (s *Sender) Send(ctx context.Context, r Report) (Outcome, error) {
	if err := r.Check(); err != nil {
		return Refused, err
	}
	body, err := json.Marshal(r)
	if err != nil {
		return Refused, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Server+"/api/v1/reports", bytes.NewReader(body))
	if err != nil {
		return Refused, err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.UserAgent != "" {
		req.Header.Set("User-Agent", s.UserAgent)
	}
	if s.Token != nil {
		if t := s.Token(); t != "" {
			req.Header.Set("Authorization", "Bearer "+t)
		}
	}
	c := s.Client
	if c == nil {
		c = defaultClient
	}
	res, err := c.Do(req)
	if err != nil {
		return Later, err
	}
	defer res.Body.Close()
	io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	switch {
	case res.StatusCode == http.StatusOK || res.StatusCode == http.StatusCreated:
		return Sent, nil
	case res.StatusCode == http.StatusBadRequest || res.StatusCode == http.StatusRequestEntityTooLarge:
		return Refused, fmt.Errorf("report: the server refused it (%d)", res.StatusCode)
	}
	return Later, fmt.Errorf("report: the server answered %d", res.StatusCode)
}

// Outbox queues reports and sends them when it can.
type Outbox struct {
	Queue  *Queue
	Sender *Sender

	mu       sync.Mutex
	flushing bool
}

// Submit queues r and starts sending the queue in the background.
func (o *Outbox) Submit(r Report) error {
	if err := o.Queue.Add(r); err != nil {
		return err
	}
	go o.Flush(context.Background())
	return nil
}

// Flush sends the queued reports, oldest first, until the queue is empty
// or one must wait (a refused report is dropped). It returns how many were sent; a Flush while another
// runs does nothing.
func (o *Outbox) Flush(ctx context.Context) (int, error) {
	o.mu.Lock()
	if o.flushing {
		o.mu.Unlock()
		return 0, nil
	}
	o.flushing = true
	o.mu.Unlock()
	defer func() {
		o.mu.Lock()
		o.flushing = false
		o.mu.Unlock()
	}()
	// Reports queued while this runs are sent too; each is tried once.
	sent, tried := 0, map[string]bool{}
	for {
		rs, err := o.Queue.Pending()
		if err != nil {
			return sent, err
		}
		var r *Report
		for i := range rs {
			if !tried[rs[i].ID] {
				r = &rs[i]
				break
			}
		}
		if r == nil {
			return sent, nil
		}
		tried[r.ID] = true
		out, err := o.Sender.Send(ctx, *r)
		switch out {
		case Later:
			return sent, err
		case Sent:
			sent++
		}
		if err := o.Queue.Remove(r.ID); err != nil {
			return sent, err
		}
	}
}

// Waiting returns how many reports are queued.
func (o *Outbox) Waiting() int {
	rs, _ := o.Queue.Pending()
	return len(rs)
}
