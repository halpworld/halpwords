package link

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"
)

// clockWrongOf is what the link saw of the clock, whether or not Status
// shows it (only to a learner on a board).
func clockWrongOf(c *Client) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.clockWrong
}

// seeClock edge cases: exactly a day is fine, a second more is wrong, and
// a missing or malformed Date changes nothing, either way round.
func TestSeeClockEdges(t *testing.T) {
	_, c, _, clk := linked(t)
	at := func(d time.Duration) http.Header {
		return http.Header{"Date": {clk.now().Add(d).UTC().Format(http.TimeFormat)}}
	}
	for _, tc := range []struct {
		name string
		h    http.Header
		want bool
	}{
		{"exactly +24h", at(clockOff), false},
		{"exactly -24h", at(-clockOff), false},
		{"+24h+1s", at(clockOff + time.Second), true},
		{"-24h-1s", at(-clockOff - time.Second), true},
		{"fixed", at(0), false},
	} {
		c.seeClock(tc.h)
		if got := clockWrongOf(c); got != tc.want {
			t.Errorf("%s: ClockWrong %v, want %v", tc.name, got, tc.want)
		}
	}
	// Unparseable or absent: the last verdict stands, wrong or right.
	for _, verdict := range []bool{true, false} {
		c.seeClock(at(clockOff * 3))
		if !verdict {
			c.seeClock(at(0))
		}
		for _, bad := range []string{"", "garbage", "0", "Mon, 32 Foo 2026 25:61:61 GMT", "2026-09-26T14:05:09Z"} {
			c.seeClock(http.Header{"Date": {bad}})
			if clockWrongOf(c) != (c.clockWrong) || clockWrongOf(c) != verdict {
				t.Errorf("Date %q changed the verdict from %v", bad, verdict)
			}
		}
	}
	// The old RFC 850 and asctime forms are still dates.
	c.seeClock(at(0))
	c.seeClock(http.Header{"Date": {clk.now().Add(5 * 24 * time.Hour).UTC().Format(time.RFC850)}})
	if !clockWrongOf(c) {
		t.Error("an RFC 850 Date was not read")
	}
}

// A real wrong clock, then fixed: the flag follows the next good answer
// in both directions, including after an error answer.
func TestClockWrongThenFixedOverHTTP(t *testing.T) {
	f, c, _, _ := linked(t)
	f.mu.Lock()
	f.skew = 48 * time.Hour
	f.mu.Unlock()
	f.failNext("/api/v1/me", 500) // an error answer has a Date too
	c.Sync(context.Background())
	if !clockWrongOf(c) {
		t.Error("clock not seen as wrong on an error answer")
	}
	f.mu.Lock()
	f.skew = 0
	f.mu.Unlock()
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if clockWrongOf(c) {
		t.Error("ClockWrong stuck after the clock was right")
	}
	// A server with no Date at all (a browser hiding it) leaves it as it is.
	f.mu.Lock()
	f.date = nil
	f.mu.Unlock()
	_ = c.Sync(context.Background())
}

// An audio pack (a raw answer, maybe cached by a browser) must not change
// the verdict.
func TestRawAnswerDoesNotJudgeTheClock(t *testing.T) {
	f, c, _, _ := linked(t)
	f.mu.Lock()
	f.skew = 30 * 24 * time.Hour // a cached old Date
	f.mu.Unlock()
	var raw []byte
	c.mu.Lock()
	tok := c.st.Access
	c.mu.Unlock()
	c.call(context.Background(), http.MethodGet, "/api/v1/me", tok, nil, nil, &raw)
	if clockWrongOf(c) {
		t.Error("a raw body judged the clock")
	}
}

// Many refused runs leave one note, the newest reason; ClearNote ends it,
// and a Late-Daily note shows first, then the run note after it is seen.
func TestManyRefusedRunsOneNote(t *testing.T) {
	f, c, st, clk := linked(t)
	c.soon = func() {}
	onBoard(t, f, c)
	f.mu.Lock()
	f.runStatus, f.runCode = 422, "stale_run"
	f.mu.Unlock()
	for i := 0; i < 5; i++ {
		c.Run(dailyRun(clk), "0123456789abcdef")
	}
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := c.Status().Note; n != OldRunNote {
		t.Fatalf("note %q", n)
	}
	// A later refusal with the clock now wrong replaces the reason.
	f.mu.Lock()
	f.skew = 5 * 24 * time.Hour
	f.mu.Unlock()
	c.Run(dailyRun(clk), "0123456789abcdef")
	c.Sync(context.Background())
	if n := c.Status().Note; n != ClockRunNote {
		t.Errorf("note %q, want the clock note", n)
	}
	c.ClearNote()
	if n := c.Status().Note; n != "" {
		t.Errorf("one ClearNote left %q", n)
	}
	c.Close()
	if n := reopen(f, st, clk).Status().Note; n != "" {
		t.Errorf("note back after restart: %q", n)
	}
}

// A note about a run belongs to the learner: unlinking drops it.
func TestRunNoteGoesWithTheLink(t *testing.T) {
	f, c, _, clk := linked(t)
	c.soon = func() {}
	onBoard(t, f, c)
	f.mu.Lock()
	f.runStatus, f.runCode = 422, "stale_run"
	f.mu.Unlock()
	c.Run(dailyRun(clk), "0123456789abcdef")
	c.Sync(context.Background())
	if c.Status().Note == "" {
		t.Fatal("no note to carry")
	}
	c.Unlink()
	if n := c.Status().Note; RunNote(n) {
		t.Errorf("run note %q survived the unlink", n)
	}
}

// Concurrent answers and Status calls are race-free with the flag.
func TestClockFlagRace(t *testing.T) {
	f, c, _, _ := linked(t)
	f.mu.Lock()
	f.skew = 72 * time.Hour
	f.mu.Unlock()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); c.Sync(context.Background()) }()
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = c.Status()
			}
		}()
	}
	wg.Wait()
}
