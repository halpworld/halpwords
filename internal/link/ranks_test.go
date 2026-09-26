package link

import (
	"context"
	"testing"

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

	// Not linked: nothing is queued.
	c.Unlink()
	c.Run(run, "0123456789abcdef")
	c.mu.Lock()
	queued := len(c.st.Runs)
	c.mu.Unlock()
	if queued != 0 || len(c.Boards()) != 0 {
		t.Error("runs or boards kept after unlinking")
	}
}
