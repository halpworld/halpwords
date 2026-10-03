package scene

import (
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/internal/typing"
	"github.com/halpworld/halpwords/pkg/words"
)

// holdKey makes only key held for the rest of the test.
func holdKey(t *testing.T, key ebiten.Key) {
	t.Helper()
	input.FakeKeys(t, func(k ebiten.Key) int {
		if k == key {
			return 1
		}
		return 0
	})
}

// Esc on a quest's introduction goes back to the hero picker; Enter begins
// (#47). On the ending, Esc still moves on.
func TestQuestIntroEscGoesBack(t *testing.T) {
	ctx := testContext(t)
	fr, _ := words.Lookup("fr")
	q := builtInQuests()[0].q
	setup := runSetup{quest: q}
	r := newRun(ctx, fr, rpg.Knight, setup)
	page := questIntro(r, setup).(*QuestPage)

	holdKey(t, ebiten.KeyEscape)
	if _, ok := page.route(ctx).(*ClassPick); !ok {
		t.Errorf("Esc on the intro goes to %T, want the hero picker", page.route(ctx))
	}
	holdKey(t, ebiten.KeyEnter)
	if _, ok := page.route(ctx).(*Crawl); !ok {
		t.Errorf("Enter on the intro goes to %T, want the dungeon", page.route(ctx))
	}

	if questEnd(r).back != nil {
		t.Error("the ending has a way back: Esc should move on")
	}
}

// The AI helper's intro fits the screen: no line is wider than its window.
func TestAIIntroFits(t *testing.T) {
	ctx := testContext(t)
	withFont(t, ctx)
	const w = 640 - 32 - 32
	lines, _ := introLines(ctx.Font, w, false)
	if len(lines) < 7 {
		t.Fatalf("%d lines", len(lines))
	}
	for _, l := range lines {
		if got := ctx.Font.Width(l, 1); got > w {
			t.Errorf("%q is %dpx wide, window text is %dpx", l, got, w)
		}
	}
	if !strings.Contains(strings.Join(lines, " "), "DeepSeek and a key.") {
		t.Errorf("text lost: %q", lines)
	}
}

// Every Hall of Fame tab says C checks a code.
func TestHallOfFameHelpHasCheck(t *testing.T) {
	ctx := rankContext(t)
	withFont(t, ctx)
	h := NewHallOfFame(ctx).(*HallOfFame)
	for mi := 0; mi < h.tabs(ctx); mi++ {
		h.mi = mi
		if !strings.Contains(h.help(ctx), "C check a") {
			t.Errorf("tab %d: %q", mi, h.help(ctx))
		}
		if w := ctx.Font.Width(h.help(ctx), 1); w > 640-16 {
			t.Errorf("tab %d: hint is %dpx wide", mi, w)
		}
	}
	if h.mi != len(fameModes) || !h.ranking() {
		t.Fatal("the Rankings tab wasn't checked")
	}
}

// While the race end screen waits for a lost connection to come back, it
// says so and Esc leaves; Enter doesn't (#47).
func TestRaceEndReconnecting(t *testing.T) {
	e := &RaceEnd{rr: &raceRun{number: 3, you: "me"}}
	racing := &link.Race{Number: 3, Racers: []link.Racer{{ID: "me", Status: link.RacerRacing}}}
	st := link.PlayState{Phase: link.PlayRejoining, Race: racing}
	if got := e.hint(st); got != "Reconnecting… Esc to leave" {
		t.Errorf("hint %q", got)
	}
	t0 := time.Now()
	if got := e.act(st, t0, false, false, true); got != raceEndStay || e.hint(st) != "Esc again to leave" {
		t.Errorf("first Esc: %v, hint %q", got, e.hint(st))
	}
	if got := e.act(st, t0.Add(time.Second), false, false, true); got != raceEndLeave {
		t.Errorf("second Esc: %v, want leave", got)
	}
	// Another key, or too long a wait, starts again.
	e.act(st, t0.Add(2*time.Second), true, false, false)
	if e.hint(st) != "Reconnecting… Esc to leave" {
		t.Errorf("after another key: %q", e.hint(st))
	}
	e.act(st, t0, false, false, true)
	if got := e.act(st, t0.Add(leaveConfirm+time.Second), false, false, true); got != raceEndStay {
		t.Errorf("Esc after the wait: %v, want stay", got)
	}
	e.asked = time.Time{}
	if got := e.act(st, t0, false, true, false); got != raceEndStay {
		t.Errorf("Enter: %v, want stay", got)
	}

	// Still racing in the room: waiting, nothing to press. Settled: back.
	st.Phase = link.PlayInRoom
	if e.hint(st) != "" || e.act(st, t0, false, true, true) != raceEndStay {
		t.Error("waiting in the room should show no hint and ignore keys")
	}
	racing.Racers[0].Status = link.RacerFinished
	if e.hint(st) != "Enter back to the room" || e.act(st, t0, false, false, true) != raceEndLobby {
		t.Error("a settled race should go back to the room")
	}
}

// The longest answer the field takes still fits the narrowest typing panel
// (the practice screen's) at the smallest text, in the widest letters.
func TestLongestAnswerFitsTypingPanel(t *testing.T) {
	ctx := testContext(t)
	withFont(t, ctx)
	const panel = game.ScreenW - 80 - 40
	for _, r := range []string{"W", "m", "Ω", "ǭ", "ᾄ"} {
		s := strings.Repeat(r, typing.MaxLen)
		if got := ctx.Font.Width(s, 1); got > panel {
			t.Errorf("%d × %q is %dpx wide, the panel is %dpx", typing.MaxLen, r, got, panel)
		}
	}
}

// The AI Helper's two footer lines sit clear of the key hint and of the
// spending panel above them, and fit across the screen (F-U4-04: the
// status line overprinted the hint). A child signed in at school is told
// who looks after the screen, not asked for a key.
func TestAISetupFooterAndSchoolWords(t *testing.T) {
	ctx := testContext(t)
	withFont(t, ctx)
	const lineH, hintY = 16 + 1, game.ScreenH - 20 // text plus its shadow
	if aiAboutY+lineH > aiStatusY || aiStatusY+lineH > hintY {
		t.Errorf("about at %d, status at %d and hint at %d overlap", aiAboutY, aiStatusY, hintY)
	}
	// The tallest panel: every provider row (Draw: y 56, 18 a row, 14
	// more), a 6 pixel gap, then 80 pixels of spending.
	if bottom := 56 + 18*aiRows + 14 + 6 + 80; bottom > aiAboutY {
		t.Errorf("the spending panel ends at %d, under the about line at %d", bottom, aiAboutY)
	}
	if w := ctx.Font.Width((&AISetup{sel: aiRowProvider}).about(ctx), 1); w > game.ScreenW-24 {
		t.Errorf("the Provider row's help is %dpx wide: it is cut off", w)
	}

	if got := aiHeading(ctx); !strings.Contains(got, "parents and teachers") {
		t.Errorf("at home the heading is %q", got)
	}
	ctx.Link = link.Open(link.Options{Store: memFiles{"link.json": []byte(`{"Way":"` + link.WayCard + `"}`)}, Server: "http://127.0.0.1:1"})
	got := aiHeading(ctx)
	if strings.Contains(got, "parents") || !strings.Contains(got, "teacher") {
		t.Errorf("at school the heading is %q", got)
	}
	if w := ctx.Font.Width(got, 1); w > game.ScreenW-16 {
		t.Errorf("the school heading is %dpx wide", w)
	}
	lines, _ := introLines(ctx.Font, game.ScreenW-64, true)
	if all := strings.Join(lines, " "); strings.Contains(all, "key") {
		t.Errorf("at school the intro asks for a key: %q", all)
	}
}
