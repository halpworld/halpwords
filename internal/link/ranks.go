package link

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/halpworld/halpwords/pkg/compete"
)

// maxRuns is how many finished runs wait to be sent at most; older ones
// are dropped first.
const maxRuns = 50

// qRun is a finished Hardcore or Daily Dungeon run waiting to be sent
// for the rankings (POST /api/v1/runs).
type qRun struct {
	Code     string // the share code
	Tally    compete.Tally
	Secs     int
	ListHash string
	At       time.Time
}

// Run queues a finished scored run to be ranked, if the game is linked.
// Only a grown-up can put the learner on a board; the server drops runs
// of learners who aren't on one. listHash is compete.ListHash of the
// run's words. It returns at once.
func (c *Client) Run(r compete.Run, listHash string) {
	if c == nil || r.Share.Score <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.st.linked() {
		return
	}
	c.st.Runs = append(c.st.Runs, qRun{Code: r.Share.Code(), Tally: r.Tally, Secs: r.Secs, ListHash: listHash, At: c.now()})
	if n := len(c.st.Runs); n > maxRuns {
		c.st.Runs = slices.Clone(c.st.Runs[n-maxRuns:])
	}
	c.saveState()
}

type wireTally struct {
	Damage    int `json:"damage"`
	Perfect   int `json:"perfect"`
	BestCombo int `json:"best_combo"`
	Bosses    int `json:"bosses"`
	Chests    int `json:"chests"`
	Misses    int `json:"misses"`
}

type wireRun struct {
	ShareCode string    `json:"share_code"`
	Tally     wireTally `json:"tally"`
	Secs      int       `json:"secs"`
	ListHash  string    `json:"list_hash"`
	PlayedAt  time.Time `json:"played_at"`
}

// uploadRuns sends the finished runs, oldest first. A run the server
// takes, or refuses for good (a 4xx: implausible, not on a board, or
// rankings off), is dropped; anything else stops until the next sync.
// syncMu is held.
func (c *Client) uploadRuns(ctx context.Context, gen int) error {
	for {
		c.mu.Lock()
		if c.gen != gen {
			c.mu.Unlock()
			return ErrNotLinked
		}
		if len(c.st.Runs) == 0 {
			c.mu.Unlock()
			return nil
		}
		r := c.st.Runs[0]
		c.mu.Unlock()

		t := r.Tally
		body := wireRun{ShareCode: r.Code, Secs: r.Secs, ListHash: r.ListHash, PlayedAt: r.At.UTC(),
			Tally: wireTally{Damage: t.Damage, Perfect: t.Perfect, BestCombo: t.BestCombo, Bosses: t.Bosses,
				Chests: t.Chests, Misses: t.Misses}}
		_, _, err := c.authed(ctx, gen, http.MethodPost, "/api/v1/runs", nil, body, nil)
		var e *Error
		if err != nil && !(errors.As(err, &e) && e.Status >= 400 && e.Status < 500 &&
			e.Status != http.StatusUnauthorized && e.Status != http.StatusTooManyRequests) {
			return err
		}
		c.mu.Lock()
		if c.gen != gen {
			c.mu.Unlock()
			return ErrNotLinked
		}
		if len(c.st.Runs) > 0 && c.st.Runs[0] == r {
			c.st.Runs = c.st.Runs[1:]
		}
		c.saveState()
		c.mu.Unlock()
	}
}

// Board is one ranking board the learner is on, as the server last sent
// it (GET /api/v1/ranks). Names are pseudonyms only.
type Board struct {
	// Scope is "friends", "class", "school", "region", "country" or
	// "world".
	Scope string `json:"scope"`
	// Name is a friend group's name; empty for other boards.
	Name string `json:"name"`
	// Kind is "hardcore" or "daily".
	Kind string `json:"kind"`
	Lang string `json:"lang"`
	// Season is "week", "month", "all" (Hardcore) or "day" (Daily
	// Dungeon); Period names it, such as "2026-W39".
	Season  string       `json:"season"`
	Period  string       `json:"period"`
	Entries []BoardEntry `json:"entries"`
}

// BoardEntry is one place on a board.
type BoardEntry struct {
	Place  int    `json:"place"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
	Score  int    `json:"score"`
	Floor  int    `json:"floor"`
	// Extra is the school's name or the country, when the board shows
	// one.
	Extra string `json:"extra,omitempty"`
	// You marks the linked learner's own place.
	You bool `json:"you,omitempty"`
}

// syncRanks fetches the boards the learner is on. A server with
// rankings off answers 404: no boards, and no error. syncMu is held.
func (c *Client) syncRanks(ctx context.Context, gen int) error {
	var body struct {
		Boards []Board `json:"boards"`
	}
	_, _, err := c.authed(ctx, gen, http.MethodGet, "/api/v1/ranks", nil, nil, &body)
	var e *Error
	if errors.As(err, &e) && e.Status == http.StatusNotFound {
		body.Boards, err = nil, nil
	}
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gen != gen {
		return ErrNotLinked
	}
	c.st.Boards = body.Boards
	return c.saveState()
}

// Boards are the ranking boards the learner is on, as the server last
// sent them: none until a grown-up puts them on one.
func (c *Client) Boards() []Board {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.st.Boards)
}
