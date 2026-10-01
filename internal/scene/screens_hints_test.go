package scene

import (
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/rpg"
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
	lines, _ := introLines(ctx.Font, w)
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
	h := NewHallOfFame(ctx).(*HallOfFame)
	for mi := 0; mi < h.tabs(ctx); mi++ {
		h.mi = mi
		if !strings.Contains(h.help(ctx), "C check") {
			t.Errorf("tab %d: %q", mi, h.help(ctx))
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
	if got := e.act(st, false, true); got != raceEndLeave {
		t.Errorf("Esc: %v, want leave", got)
	}
	if got := e.act(st, true, false); got != raceEndStay {
		t.Errorf("Enter: %v, want stay", got)
	}

	// Still racing in the room: waiting, nothing to press. Settled: back.
	st.Phase = link.PlayInRoom
	if e.hint(st) != "" || e.act(st, true, true) != raceEndStay {
		t.Error("waiting in the room should show no hint and ignore keys")
	}
	racing.Racers[0].Status = link.RacerFinished
	if e.hint(st) != "Enter back to the room" || e.act(st, false, true) != raceEndLobby {
		t.Error("a settled race should go back to the room")
	}
}
