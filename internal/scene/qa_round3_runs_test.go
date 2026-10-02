package scene

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/pkg/compete"
	"github.com/halpworld/halpwords/pkg/words"
)

func firstPerfects(r *run) int {
	n := 0
	for _, l := range r.log {
		if strings.HasPrefix(l.text, "First perfect") {
			n++
		}
	}
	return n
}

// Fall after spelling new words perfectly: the XP goes with the hero, and
// spelling them again after waking gives it again, exactly once.
func TestQARound3PerfectXPAgainAfterWake(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	c.run.perfect[1] = true
	c.pray(ctx)
	perf := words.Result{Tier: words.Perfect}
	c.scoreAnswer(5, perf, "", false, 1)
	c.scoreAnswer(6, perf, "", false, 1)
	c.scoreAnswer(5, perf, "", false, 1) // not twice
	if n := firstPerfects(c.run); n != 2 {
		t.Fatalf("before the fall: %d first perfects, want 2", n)
	}
	c.die()
	w := c.woken(ctx)
	if w.run.perfect[5] || w.run.perfect[6] || !w.run.perfect[1] {
		t.Fatalf("woke with perfect %v", w.run.perfect)
	}
	was := firstPerfects(w.run)
	before := w.run.hero.XP
	lvl := w.run.hero.Level
	w.scoreAnswer(5, perf, "", false, 1)
	w.scoreAnswer(5, perf, "", false, 1)
	w.scoreAnswer(1, perf, "", false, 1) // already perfect at the shrine
	if w.run.hero.XP+w.run.hero.Level*1000 <= before+lvl*1000 {
		t.Fatalf("no XP the second time: %d -> %d", before, w.run.hero.XP)
	}
	got := firstPerfects(w.run) - was
	if got != 1 {
		t.Fatalf("after waking: %d first perfects for word 5 and 1, want 1", got)
	}
	// A second fall, with no new shrine, forgets nothing it shouldn't.
	w.die()
	w2 := w.woken(ctx)
	if w2.run.perfect[5] || !w2.run.perfect[1] {
		t.Fatalf("second wake: %v", w2.run.perfect)
	}
}

// A save from before shrines kept Perfect has the top-level Perfect list
// only: loading keeps those words, and they earn nothing twice.
func TestQARound3OldSaveJSONWithoutShrinePerfect(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	r, l := testRun(t, ctx)
	data, err := encodeSave(r, l, l.Start, dungeon.South, true)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	delete(m["Shrine"].(map[string]any), "Perfect")
	if sus, ok := m["Suspend"].(map[string]any); ok {
		delete(sus, "Perfect")
	}
	data, _ = json.Marshal(m)
	if strings.Contains(string(data), `"Shrine":{`) && strings.Contains(string(data), `"Perfect":null`) {
		t.Fatal("test setup: Perfect still present")
	}
	got, err := decodeSave(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	if !got.run.perfect[3] {
		t.Fatalf("loaded perfect %v", got.run.perfect)
	}
	if len(got.run.shrine.Perfect) != 1 || got.run.shrine.Perfect[0] != 3 {
		t.Fatalf("shrine perfect %v, want [3]", got.run.shrine.Perfect)
	}
	c := crawlOn(got.run, got.level)
	c.scoreAnswer(3, words.Result{Tier: words.Perfect}, "", false, 1)
	if n := firstPerfects(c.run); n != 0 {
		t.Fatal("word 3 paid first-perfect XP again after an old save")
	}
}

// A shrine save with Perfect present but empty ([]) is a real empty set: a
// fall must not bring back words from after the shrine.
func TestQARound3EmptyShrinePerfectIsKept(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	c.pray(ctx) // no perfect words yet
	c.run.perfect[9] = true
	c.die()
	if p := c.woken(ctx).run.perfect; len(p) != 0 {
		t.Fatalf("woke with %v, want none", p)
	}
	loaded, err := loadCrawl(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.run.perfect) != 0 {
		t.Fatalf("the save has %v, want none", loaded.run.perfect)
	}
}

func qaDailyCrawl(t *testing.T, ctx *game.Context, now time.Time) *Crawl {
	t.Helper()
	fr, _ := words.Lookup("fr")
	old := runNow
	t.Cleanup(func() { runNow = old })
	runNow = func() time.Time { return now }
	r := newRun(ctx, fr, rpg.Rogue, dailySetup(ctx, fr))
	r.sound = &game.Sound{Muted: true}
	l := dungeon.Generate(r.floorSeed(1), 1)
	return &Crawl{run: r, level: l, pos: l.Start, facing: l.StartDir}
}

// A suspended Daily keeps its day; resumed the same day it is ranked and
// sent once, resumed on a later day it is not sent and says so.
func TestQARound3SuspendedDailyResume(t *testing.T) {
	ctx := testContext(t)
	day := time.Date(2026, time.September, 24, 21, 0, 0, 0, time.Local)
	c := qaDailyCrawl(t, ctx, day)
	data, err := encodeSave(c.run, c.level, c.pos, c.facing, true)
	if err != nil {
		t.Fatal(err)
	}
	var sent []compete.Run
	old := sendRun
	t.Cleanup(func() { sendRun = old })
	sendRun = func(_ *game.Context, r compete.Run, _ string) { sent = append(sent, r) }
	for _, k := range []struct {
		name  string
		now   time.Time
		sends int
	}{
		{"same day", day.Add(2 * time.Hour), 1},
		{"next day", day.Add(4 * time.Hour), 0},
		{"a week later", day.AddDate(0, 0, 7), 0},
	} {
		sent = nil
		got, err := decodeSave(ctx, data)
		if err != nil {
			t.Fatal(err)
		}
		if got.run.mode != compete.Daily || got.run.day != "2026-09-24" {
			t.Fatalf("%s: loaded %v %q", k.name, got.run.mode, got.run.day)
		}
		got.run.sound = &game.Sound{Muted: true}
		got.run.tally.Damage = 30
		runNow = func() time.Time { return k.now }
		g := newGameOver(ctx, got.run, true)
		if len(sent) != k.sends || g.unranked != (k.sends == 0) {
			t.Errorf("%s: %d sent, unranked %v", k.name, len(sent), g.unranked)
		}
	}
}

// A Daily whose day is empty (an old save) is never sent, as a Daily or
// as anything else: no seed share and no Hardcore share.
func TestQARound3EmptyDayDailyNeverSent(t *testing.T) {
	ctx := testContext(t)
	c := qaDailyCrawl(t, ctx, time.Date(2026, time.September, 24, 9, 0, 0, 0, time.Local))
	data, _ := encodeSave(c.run, c.level, c.pos, c.facing, true)
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	delete(m, "Day")
	data, _ = json.Marshal(m)
	got, err := decodeSave(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	var sent []compete.Run
	old := sendRun
	t.Cleanup(func() { sendRun = old })
	sendRun = func(_ *game.Context, r compete.Run, _ string) { sent = append(sent, r) }
	got.run.sound = &game.Sound{Muted: true}
	got.run.tally.Damage = 30
	g := newGameOver(ctx, got.run, true)
	if len(sent) != 0 || !g.unranked {
		t.Fatalf("empty day: %d sent, unranked %v", len(sent), g.unranked)
	}
	if got.run.ranked(time.Time{}) {
		t.Fatal("empty day ranks")
	}
}

// Quit to title right after a shrine save, a load and a prayer does not
// warn, however long it was played; a real change does.
func TestQARound3QuitWarnsOnlyOnRealChange(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	c.run.onDisk = true
	c.pray(ctx)
	for i := 0; i < 300; i++ {
		c.countPlayed()
	}
	c.mode = modeExplore
	c.pause(ctx)
	if c.unsaved {
		t.Fatal("quit would warn right after praying")
	}
	c.unpause(ctx)
	c.pray(ctx) // praying again, with only the message log changed
	c.pause(ctx)
	if c.unsaved {
		t.Fatal("quit would warn after praying twice")
	}
	c.unpause(ctx)
	loaded, err := loadCrawl(ctx)
	if err != nil {
		t.Fatal(err)
	}
	loaded.run.sound = &game.Sound{Muted: true}
	loaded.run.played += 100
	loaded.pause(ctx)
	if loaded.unsaved {
		t.Fatal("quit would warn right after loading")
	}
	loaded.unpause(ctx)
	loaded.run.hero.Gold += 3
	loaded.pause(ctx)
	if !loaded.unsaved {
		t.Fatal("quit would not warn after earning gold")
	}
	loaded.unpause(ctx)
	loaded.run.perfect[11] = true
	loaded.writeSave(ctx, false)
	loaded.run.perfect[12] = true
	loaded.pause(ctx)
	if !loaded.unsaved {
		t.Fatal("a new perfect word is not an unsaved change")
	}
}

// The save written by a prayer survives the key/value store the web game
// uses (whole files, bytes in and out): Perfect comes back after Read.
func TestQARound3PerfectSurvivesStoredBytes(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	c.run.perfect[2], c.run.perfect[40] = true, true
	c.pray(ctx)
	c.run.perfect[41] = true
	data, err := encodeSave(c.run, c.level, c.pos, c.facing, true)
	if err != nil {
		t.Fatal(err)
	}
	// What a string-only store does to the bytes.
	data = []byte(string(data))
	got, err := decodeSave(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	if !got.run.perfect[41] || len(got.run.shrine.Perfect) != 2 {
		t.Fatalf("perfect %v, shrine %v", got.run.perfect, got.run.shrine.Perfect)
	}
	// Wake from that very save: back to the shrine's two words.
	g := crawlOn(got.run, got.level)
	g.die()
	if p := g.woken(ctx).run.perfect; len(p) != 2 || !p[2] || !p[40] {
		t.Fatalf("woke with %v", p)
	}
}
