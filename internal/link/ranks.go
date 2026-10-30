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

// Run queues a finished scored run to be ranked, if the game is linked
// and the learner's progress is kept.
// Only a grown-up can put the learner on a board; the server drops runs
// of learners who aren't on one. A run that cleared no floor
// (compete.Run.Ranked) isn't queued: no board takes it. listHash is
// compete.ListHash of the run's words. It returns at once.
func (c *Client) Run(r compete.Run, listHash string) {
	if c == nil || r.Share.Score <= 0 || !r.Ranked() {
		return
	}
	c.mu.Lock()
	if !c.st.linked() || c.st.Stopped {
		c.mu.Unlock()
		return
	}
	c.st.Runs = append(c.st.Runs, qRun{Code: r.Share.Code(), Tally: r.Tally, Secs: r.Secs, ListHash: listHash,
		// Without the monotonic reading, so time asleep counts towards
		// compete.DailySendWindow, as it does on the server.
		At: c.now().Round(0)})
	if n := len(c.st.Runs); n > maxRuns {
		c.st.Runs = slices.Clone(c.st.Runs[n-maxRuns:])
	}
	c.saveState()
	c.mu.Unlock()
	if r.Share.Daily {
		// A Daily run only counts if the server has it within
		// compete.DailySendWindow (Q127): send it now, not at the next
		// sync.
		if c.soon != nil {
			c.soon()
		} else {
			c.SyncNow()
		}
	}
}

// codeDailyLate is the server's answer to a Daily Dungeon run it received
// more than compete.DailySendWindow after the run ended (Q127).
const codeDailyLate = "daily_late"

// LateNote is the note (Status.Note) after a Daily run was dropped for being too late.
const LateNote = "Your Daily run reached the board too late to count. Your XP is safe!"

// The server's answers to a run it refuses for its time: stale_run is
// older than its limit (14 days) or ahead of the server, implausible_run
// fails its checks, which a Daily played on the wrong day by a wrong
// clock does.
const (
	codeStaleRun       = "stale_run"
	codeImplausibleRun = "implausible_run"
)

// The notes (Status.Note) after the server refused a run (runNote).
const (
	ClockRunNote = "A run didn't count: the clock on this computer looks wrong."
	OldRunNote   = "A run was too old to count on the board. Your XP is safe!"
	BadRunNote   = "A run couldn't count on the board. Your XP is safe!"
)

// RunNote reports whether note is one of the notes about a run that
// didn't count on a board (LateNote and runNote's).
func RunNote(note string) bool {
	switch note {
	case LateNote, ClockRunNote, OldRunNote, BadRunNote:
		return true
	}
	return false
}

// runNote is the note for a run the server refused with code, or "" for
// a refusal that says nothing to the player. A wrong clock (clockWrong)
// is named first: it is the likely cause, and the one the player can
// fix.
func runNote(code string, clockWrong bool) string {
	switch {
	case code != codeStaleRun && code != codeImplausibleRun:
		return ""
	case clockWrong:
		return ClockRunNote
	case code == codeStaleRun:
		return OldRunNote
	}
	return BadRunNote
}

// daily reports whether r is a Daily Dungeon run.
func (r qRun) daily() bool {
	s, err := compete.ParseShare(r.Code)
	return err == nil && s.Daily
}

// late reports whether r is a Daily Dungeon run past
// compete.DailySendWindow at now: the server would refuse it. Both times
// are the device's clock, as age_s is, so a wrong clock cancels out.
func (r qRun) late(now time.Time) bool {
	return r.daily() && compete.DailyTooLate(r.At, now)
}

// onDailyBoard reports whether the learner is on a Daily Dungeon board,
// as the server last said. c.mu is held.
func (c *Client) onDailyBoard() bool {
	return slices.ContainsFunc(c.st.Boards, func(b Board) bool { return b.Kind == "daily" })
}

// DailyLeft reports how long the Daily Dungeon run with share code code
// has left to reach the server and count (compete.DailySendWindow), and
// whether it still waits to be sent, in time, while the game is linked,
// syncing normally and the learner is on a Daily board: the result
// screen then asks the child to go online. It says nothing for a game
// that isn't linked, a paused account, a learner on no Daily board
// (rankings off), or while a sync is under way.
func (c *Client) DailyLeft(code string) (time.Duration, bool) {
	if c == nil {
		return 0, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.st.linked() || c.st.Stopped || c.suspended || c.busy || !c.onDailyBoard() {
		return 0, false
	}
	now := c.now()
	for _, r := range c.st.Runs {
		if r.Code == code && r.daily() && !r.late(now) {
			return compete.DailySendWindow - now.Sub(r.At), true
		}
	}
	return 0, false
}

// DailyWaiting reports whether DailyLeft would warn about code.
func (c *Client) DailyWaiting(code string) bool {
	_, ok := c.DailyLeft(code)
	return ok
}

// dailyWaits reports whether a Daily run waits in time to be sent, so
// the next sync must not wait long. c.mu is held.
func (c *Client) dailyWaits() bool {
	now := c.now()
	for _, r := range c.st.Runs {
		if r.daily() && !r.late(now) {
			return true
		}
	}
	return false
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
	// AgeS is how long ago the run ended, in whole seconds on the
	// device's own clock (now - PlayedAt, at least 0), so a device clock
	// that is wrong doesn't make a Daily run late (Q127): the server
	// takes its own now - AgeS as the time the run ended.
	AgeS int64 `json:"age_s"`
}

// errCode is e's code, or "" for none.
func errCode(e *Error) string {
	if e == nil {
		return ""
	}
	return e.Code
}

// ageSecs is how long ago, in whole seconds on the device clock, a run
// that ended at at is at now; never below 0.
func ageSecs(at, now time.Time) int64 {
	return max(0, int64(now.Sub(at)/time.Second))
}

// uploadRuns sends the finished runs, oldest first. A run the server
// takes, or refuses for good (a 4xx: implausible, not on a board, or
// rankings off), is dropped; anything else stops until the next sync.
// A Daily run past compete.DailySendWindow is dropped without being sent,
// and one the server calls too late (daily_late) is dropped too; either
// way a learner on a Daily board gets LateNote. A run refused for its time
// or its checks (stale_run, implausible_run) gives a learner on a board
// a note saying so (runNote), naming a wrong clock when the server's
// Date says so (ClockWrong). Only the board loses it: the run's XP and
// word practice were kept when it ended. syncMu is held.
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
		now := c.now()
		if r.late(now) {
			c.st.Runs = c.st.Runs[1:]
			c.st.LateDaily = c.st.LateDaily || c.onDailyBoard()
			c.saveState()
			c.mu.Unlock()
			continue
		}
		c.mu.Unlock()

		t := r.Tally
		body := wireRun{ShareCode: r.Code, Secs: r.Secs, ListHash: r.ListHash, PlayedAt: r.At.UTC(), AgeS: ageSecs(r.At, now),
			Tally: wireTally{Damage: t.Damage, Perfect: t.Perfect, BestCombo: t.BestCombo, Bosses: t.Bosses,
				Chests: t.Chests, Misses: t.Misses}}
		_, _, err := c.authed(ctx, gen, http.MethodPost, "/api/v1/runs", nil, body, nil)
		var e *Error
		// A paused account (halpwords-server#130) keeps the run for later.
		if err != nil && (isSuspended(err) || !(errors.As(err, &e) && e.Status >= 400 && e.Status < 500 &&
			e.Status != http.StatusUnauthorized && e.Status != http.StatusTooManyRequests)) {
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
		if e != nil && e.Code == codeDailyLate && c.onDailyBoard() {
			c.st.LateDaily = true
		}
		if n := runNote(errCode(e), c.clockWrong); n != "" && len(c.st.Boards) > 0 {
			c.st.RunNote = n
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
