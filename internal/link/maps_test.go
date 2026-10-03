package link

import (
	"context"
	"io/fs"
	"testing"
	"time"

	"github.com/halpworld/halpwords/assets"
)

// questText is a quest that passes maps' checks: the one built into the
// game.
func questText(t *testing.T) string {
	t.Helper()
	data, err := fs.ReadFile(assets.Quests, "quests/scribes-cellars.hwquest")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// mapsBody is a quest as GET /api/v1/maps sends it (the server's
// docs/api/maps.response.schema.json).
func mapsBody(id, text, due string) map[string]any {
	b := map[string]any{"id": id, "map_id": "map_1", "version": 2, "title": "Kit's quest", "language": "",
		"floors": 3, "text": text}
	if due != "" {
		b["due_at"] = due
	}
	return b
}

// Quests a grown-up gives the learner in the map editor reach the linked
// game at its next sync, are kept with the link, go when they are ended,
// and a quest the game can't play is left out (F-U3-05,
// halpworld/halpwords#34).
func TestAssignedQuestsSync(t *testing.T) {
	f, c, _, _ := linked(t)
	if qs := c.AssignedQuests(); len(qs) != 0 {
		t.Fatalf("quests before any were given: %v", qs)
	}
	text := questText(t)
	f.mu.Lock()
	f.maps = []map[string]any{mapsBody("asg_q1", text, "2026-10-02T23:59:59Z"), mapsBody("asg_bad", "{nonsense", "")}
	f.mapsETag = `"m1"`
	f.mu.Unlock()
	before := c.Changes()
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	qs := c.AssignedQuests()
	if len(qs) != 1 || qs[0].ID != "asg_q1" || qs[0].Title != "Kit's quest" || qs[0].Version != 2 {
		t.Fatalf("quests: %+v", qs)
	}
	if c.Changes() == before {
		t.Error("no change for the game to pick up")
	}
	q, err := qs[0].Quest()
	if err != nil || q.Title != "The Scribe's Cellars" || len(q.Maps) == 0 {
		t.Fatalf("quest: %v %v", q, err)
	}
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	if got := qs[0].WhenText(now); got != "due tomorrow" {
		t.Errorf("WhenText = %q", got)
	}
	// Kept with the link.
	if again := Open(c.o); len(again.AssignedQuests()) != 1 {
		t.Error("quests weren't kept")
	}
	// Unchanged: the ETag saves sending them again.
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(c.AssignedQuests()) != 1 {
		t.Error("lost on a 304")
	}
	// Ended on the website.
	f.mu.Lock()
	f.maps, f.mapsETag = nil, `"m2"`
	f.mu.Unlock()
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if qs := c.AssignedQuests(); len(qs) != 0 {
		t.Errorf("ended quest kept: %v", qs)
	}
}

// A server from before quests could be given (404) has none, and the
// sync doesn't fail; unlinking forgets the quests.
func TestAssignedQuestsOldServerAndUnlink(t *testing.T) {
	f, c, _, _ := linked(t)
	f.mu.Lock()
	f.noMaps = true
	f.mu.Unlock()
	if err := c.Sync(context.Background()); err != nil {
		t.Fatalf("sync with an older server: %v", err)
	}
	f.mu.Lock()
	f.noMaps, f.maps, f.mapsETag = false, []map[string]any{mapsBody("asg_q1", questText(t), "")}, `"m1"`
	f.mu.Unlock()
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(c.AssignedQuests()) != 1 {
		t.Fatal("no quest")
	}
	c.Unlink()
	if qs := c.AssignedQuests(); len(qs) != 0 {
		t.Errorf("quests kept after unlinking: %v", qs)
	}
}
