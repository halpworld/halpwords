package link

import (
	"context"
	"testing"
	"time"

	"github.com/halpworld/halpwords/pkg/compete"
)

// TestRunsAndBoards: a finished run is sent once, with its tally, and
// the boards come back; a refused run is dropped, not sent again.
func TestRunsAndBoards(t *testing.T) {
	f, c, _, _ := linked(t)
	if len(c.Boards()) != 0 {
		t.Error("boards from a server with rankings off")
	}
	tally := compete.Tally{Damage: 300, Perfect: 10, BestCombo: 4, Bosses: 1, Chests: 2, Misses: 1}
	run := compete.Run{Share: compete.Share{Lang: "fr", Seed: 42, Floor: 3, Score: tally.Score(3)}, Tally: tally, Secs: 200}
	c.Run(run, "0123456789abcdef")
	f.mu.Lock()
	f.boards = []any{map[string]any{"scope": "class", "kind": "hardcore", "lang": "fr", "season": "week",
		"period": "2026-W39", "entries": []any{map[string]any{"place": 1, "name": "Brave Otter 12", "score": run.Share.Score,
			"floor": 3, "you": true}}}}
	f.mu.Unlock()
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	runs := f.runs
	f.mu.Unlock()
	if len(runs) != 1 || runs[0]["share_code"] != run.Share.Code() || runs[0]["list_hash"] != "0123456789abcdef" ||
		runs[0]["secs"] != float64(200) || runs[0]["tally"].(map[string]any)["best_combo"] != float64(4) {
		t.Fatalf("runs sent: %v", runs)
	}
	b := c.Boards()
	if len(b) != 1 || b[0].Scope != "class" || len(b[0].Entries) != 1 || !b[0].Entries[0].You {
		t.Fatalf("boards: %+v", b)
	}

	// A refused run is dropped.
	f.mu.Lock()
	f.runStatus = 422
	f.mu.Unlock()
	c.Run(run, "0123456789abcdef")
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	n := len(f.runs)
	f.mu.Unlock()
	if n != 2 {
		t.Errorf("%d runs sent, want 2", n)
	}

	// A run that cleared no floor is not queued (#93).
	idle := compete.Run{Share: compete.Share{Lang: "fr", Seed: 43, Floor: 1, Score: tally.Score(1)}, Tally: tally, Secs: 200}
	c.Run(idle, "0123456789abcdef")
	c.mu.Lock()
	queued := len(c.st.Runs)
	c.mu.Unlock()
	if queued != 0 {
		t.Error("a floor-1 run was queued")
	}

	// Not linked: nothing is queued.
	c.Unlink()
	c.Run(run, "0123456789abcdef")
	c.mu.Lock()
	queued = len(c.st.Runs)
	c.mu.Unlock()
	if queued != 0 || len(c.Boards()) != 0 {
		t.Error("runs or boards kept after unlinking")
	}
}

// dailyRun is a Daily Dungeon run on clk's day that cleared two floors.
func dailyRun(clk *clock) compete.Run {
	tally := compete.Tally{Damage: 120, Perfect: 4, BestCombo: 3}
	d := clk.now()
	return compete.Run{Share: compete.Share{Lang: "fr", Daily: true, Month: d.Month(), Day: d.Day(), Floor: 2,
		Score: tally.Score(2)}, Tally: tally, Secs: 120}
}

// onBoard puts the learner on a Daily board, so the result screen warns.
func onBoard(t *testing.T, f *fake, c *Client) {
	t.Helper()
	f.mu.Lock()
	f.boards = []any{map[string]any{"scope": "class", "kind": "daily", "lang": "fr", "season": "day",
		"period": "2026-09-26", "entries": []any{}}}
	f.mu.Unlock()
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// TestDailySendWindow (Q127): a Daily run asks for a sync at once; it
// waits to be sent only within compete.DailySendWindow, and past it is
// dropped without being sent, with a note that outlives a restart.
// Hardcore runs keep waiting.
func TestDailySendWindow(t *testing.T) {
	f, c, st, clk := linked(t)
	kicks := 0
	c.soon = func() { kicks++ }
	run := dailyRun(clk)
	code := run.Share.Code()

	// Not on a board (rankings off): queued, but no warning.
	c.Run(run, "0123456789abcdef")
	if kicks != 1 {
		t.Errorf("%d syncs asked for after a Daily run, want 1", kicks)
	}
	if c.DailyWaiting(code) {
		t.Error("warned with no boards")
	}
	c.mu.Lock()
	c.st.Runs = nil
	c.mu.Unlock()
	onBoard(t, f, c)

	// Offline: the run waits, and the game says so.
	c.Run(run, "0123456789abcdef")
	if !c.DailyWaiting(code) {
		t.Fatal("a queued Daily run is not waiting")
	}
	c.mu.Lock()
	c.failures = 9 // offline for a while
	c.mu.Unlock()
	if w := c.wait(); w != SyncEvery {
		t.Errorf("wait with a Daily run waiting = %v, want %v", w, SyncEvery)
	}
	clk.add(compete.DailySendWindow)
	if !c.DailyWaiting(code) {
		t.Error("not waiting at exactly the window")
	}
	hardcore := compete.Run{Share: compete.Share{Lang: "fr", Seed: 7, Floor: 2, Score: run.Share.Score}, Tally: run.Tally, Secs: 120}
	c.Run(hardcore, "0123456789abcdef")
	if kicks != 2 {
		t.Errorf("a Hardcore run asked for a sync (%d)", kicks)
	}
	clk.add(time.Second)
	if c.DailyWaiting(code) {
		t.Error("still waiting past the window")
	}

	// Back online: the late Daily run is dropped unsent, the Hardcore run
	// is sent, and the note is shown until the player has seen it.
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	runs := f.runs
	f.mu.Unlock()
	if len(runs) != 1 || runs[0]["share_code"] != hardcore.Share.Code() {
		t.Fatalf("runs sent: %v", runs)
	}
	// age_s is the device-clock time since the run ended (1 s here).
	if runs[0]["age_s"] != float64(1) {
		t.Errorf("age_s = %v, want 1", runs[0]["age_s"])
	}
	if n := c.Status().Note; n != LateNote {
		t.Fatalf("note %q", n)
	}
	// It outlives a restart.
	c2 := Open(Options{Store: st, Server: f.srv.URL, Version: "v1.2.0-3-gabcdef", OwnDir: "words", Now: clk.now})
	if n := c2.Status().Note; n != LateNote {
		t.Fatalf("note after a restart %q", n)
	}
	c2.ClearNote()
	if n := c2.Status().Note; n != "" {
		t.Fatalf("note after ClearNote %q", n)
	}
	c3 := Open(Options{Store: st, Server: f.srv.URL, Version: "v1.2.0-3-gabcdef", OwnDir: "words", Now: clk.now})
	if n := c3.Status().Note; n != "" {
		t.Fatalf("note came back after a restart: %q", n)
	}
}

// TestDailyLateFromServer (Q127): the server's daily_late answer drops
// the run for good and tells the player; other runs are still sent.
func TestDailyLateFromServer(t *testing.T) {
	f, c, _, clk := linked(t)
	c.soon = func() {}
	onBoard(t, f, c)
	f.mu.Lock()
	f.runStatus, f.runCode = 422, "daily_late"
	f.mu.Unlock()
	run := dailyRun(clk)
	c.Run(run, "0123456789abcdef")
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	n := len(f.runs)
	f.mu.Unlock()
	if n != 1 {
		t.Errorf("%d tries, want 1", n)
	}
	if c.DailyWaiting(run.Share.Code()) {
		t.Error("still waiting after daily_late")
	}
	if s := c.Status().Note; s != LateNote {
		t.Errorf("note %q", s)
	}

	// Another refusal is dropped quietly, as before.
	c.ClearNote()
	f.mu.Lock()
	f.runCode = ""
	f.mu.Unlock()
	c.Run(run, "0123456789abcdef")
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s := c.Status().Note; s != "" {
		t.Errorf("note after implausible_run %q", s)
	}
}

// TestRunAge: age_s is whole seconds on the device clock and never
// negative, so a device clock set back can't send a run from the future.
func TestRunAge(t *testing.T) {
	end := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		after time.Duration
		want  int64
	}{{0, 0}, {999 * time.Millisecond, 0}, {90 * time.Second, 90}, {compete.DailySendWindow, 900}, {-time.Hour, 0}} {
		if got := ageSecs(end, end.Add(c.after)); got != c.want {
			t.Errorf("ageSecs after %v = %d, want %d", c.after, got, c.want)
		}
	}
}

// TestDailyLeft: the time left counts down; a paused account, a sync
// under way or a learner on no Daily board gets no warning, and a late
// run on no Daily board is dropped without a note.
func TestDailyLeft(t *testing.T) {
	f, c, _, clk := linked(t)
	c.soon = func() {}
	onBoard(t, f, c)
	run := dailyRun(clk)
	code := run.Share.Code()
	c.Run(run, "0123456789abcdef")
	clk.add(3 * time.Minute)
	if left, ok := c.DailyLeft(code); !ok || left != 12*time.Minute {
		t.Errorf("DailyLeft = %v, %v; want 12m, true", left, ok)
	}
	for _, set := range []func(bool){
		func(v bool) { c.suspended = v },
		func(v bool) { c.busy = v },
	} {
		c.mu.Lock()
		set(true)
		c.mu.Unlock()
		if c.DailyWaiting(code) {
			t.Error("warned while paused or syncing")
		}
		c.mu.Lock()
		set(false)
		c.mu.Unlock()
	}

	// Only a Hardcore board: no warning, and no note when it is dropped.
	f.mu.Lock()
	f.boards = []any{map[string]any{"scope": "class", "kind": "hardcore", "lang": "fr", "season": "week",
		"period": "2026-W39", "entries": []any{}}}
	f.mu.Unlock()
	c.mu.Lock()
	c.st.Boards = []Board{{Scope: "class", Kind: "hardcore"}}
	c.mu.Unlock()
	if c.DailyWaiting(code) {
		t.Error("warned with no Daily board")
	}
	clk.add(compete.DailySendWindow)
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	n := len(c.st.Runs)
	c.mu.Unlock()
	if n != 0 || c.Status().Note != "" {
		t.Errorf("%d runs left, note %q", n, c.Status().Note)
	}
}

// TestRunTimeHasNoMonotonicReading: the end of a run is kept as wall
// time, so time the computer slept counts towards the window.
func TestRunTimeHasNoMonotonicReading(t *testing.T) {
	_, c, _, clk := linked(t)
	c.soon = func() {}
	run := dailyRun(clk)
	c.now = time.Now // carries a monotonic reading
	c.Run(run, "0123456789abcdef")
	c.mu.Lock()
	at := c.st.Runs[0].At
	c.mu.Unlock()
	if at != at.Round(0) {
		t.Errorf("At %v keeps a monotonic reading", at)
	}
}
