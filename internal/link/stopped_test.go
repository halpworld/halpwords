package link

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/halpworld/halpwords/pkg/gameai"
	"github.com/halpworld/halpwords/pkg/words"
)

// TestStopDropsFilesAtOnce (G1, PLAN §14 2026-10-12): stopping removes
// the queue file at once, not at the next save, and drops what was
// parked for this learner after an earlier link; another learner's
// parked events stay.
func TestStopDropsFilesAtOnce(t *testing.T) {
	f, c, st, clk := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	if err := c.Save(); err != nil || !st.has(queueFile) {
		t.Fatalf("queue not saved: %v", err)
	}
	c.mu.Lock()
	me := c.st.Me.Learner.ID
	q := queue{Answers: []qAnswer{{Seq: 900}}}
	c.parked = []parked{{Learner: me, At: clk.now(), Queue: q}, {Learner: "lrn_other", At: clk.now(), Queue: q}}
	c.writeParked()
	c.mu.Unlock()

	f.stop(true)
	if err := c.Sync(context.Background()); !errors.Is(err, ErrProgressStopped) {
		t.Fatalf("sync: %v", err)
	}
	if st.has(queueFile) {
		t.Error("queue file kept after stopping")
	}
	var ps []parked
	data, _ := st.Read(parkedFile)
	json.Unmarshal(data, &ps)
	if len(ps) != 1 || ps[0].Learner != "lrn_other" {
		t.Errorf("parked after stopping: %+v", ps)
	}
}

// TestStoppedOnOpenDropsQueue (G1): a queue file next to a stopped
// link (left by a game that stopped before it saved, or by an older
// game) is dropped when the game starts, and never sent.
func TestStoppedOnOpenDropsQueue(t *testing.T) {
	f, c, st, clk := linked(t)
	f.stop(true)
	c.Sync(context.Background())
	c.Save()
	if err := st.Write(queueFile, []byte(`{"Answers":[{"Seq":901}]}`)); err != nil {
		t.Fatal(err)
	}
	c2 := reopen(f, st, clk)
	if s := c2.Status(); !s.Stopped || s.Pending != 0 || st.has(queueFile) {
		t.Fatalf("after opening: %+v, file %v", s, st.has(queueFile))
	}
	f.stop(false)
	if err := c2.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(f.eventsOf("answers")); n != 0 {
		t.Errorf("%d answers from the stopped time sent", n)
	}
}

// TestStoppedAsksNoAIOrPlay (G2): while stopped the game asks neither
// Halpwords AI nor for a play ticket, and a room it is in is left.
func TestStoppedAsksNoAIOrPlay(t *testing.T) {
	f, c, _, _ := linked(t)
	f.mu.Lock()
	f.me["ai"] = map[string]any{"available": true, "tasks": gameai.Tasks}
	f.ai = func(string, []byte) (int, any) { return 200, map[string]any{} }
	f.mu.Unlock()
	if err := c.Sync(context.Background()); err != nil || !c.AIAvailable() {
		t.Fatalf("AI before stopping: %v", err)
	}
	f.stop(true)
	c.Sync(context.Background())
	if c.AIAvailable() {
		t.Error("AI available while stopped")
	}
	before := f.count("/api/v1/ai/" + gameai.Taunts)
	if err := c.AskAI(context.Background(), gameai.Taunts, gameai.WordsRequest{}, nil); !errors.Is(err, ErrNoAI) {
		t.Errorf("AskAI while stopped: %v", err)
	}
	if f.count("/api/v1/ai/"+gameai.Taunts) != before {
		t.Error("asked AI while stopped")
	}
	p := c.Play()
	p.Join("ABCDEF")
	if s := p.State(); s.Phase != PlayOff || s.Problem != playStopped {
		t.Errorf("join while stopped: %+v", s)
	}
	if f.count("/api/v1/play/tickets") != 0 {
		t.Error("asked for a play ticket while stopped")
	}

	// In a room when the stop comes: it is left.
	p.mu.Lock()
	p.st.Phase = PlayInRoom
	p.mu.Unlock()
	c.mu.Lock()
	c.st.Stopped = false
	c.stopProgress()
	c.mu.Unlock()
	deadline := time.Now().Add(2 * time.Second)
	for p.State().Phase != PlayOff && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if s := p.State(); s.Phase != PlayOff || s.Problem != playStopped {
		t.Errorf("in a room when stopped: %+v", s)
	}
}

// TestChoicesClearsStopped (G3): when My choices says the progress is
// kept, the game stops waiting for a sync and queues again at once.
func TestChoicesClearsStopped(t *testing.T) {
	f, c, _, _ := linked(t)
	cs := &choicesServer{stopped: true}
	f.mu.Lock()
	f.choices = cs.serve(f)
	f.mu.Unlock()
	f.stop(true)
	c.Sync(context.Background())
	cs.stopped = false
	f.stop(false)
	if _, err := c.Choices(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.Status().Stopped {
		t.Error("still stopped after the server said it isn't")
	}
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	if s := c.Status(); s.Pending == 0 && len(f.eventsOf("answers")) == 0 {
		t.Error("not queuing once kept again")
	}
}

// TestAskedTooOften: 429 asked_too_often is ErrAskedTooOften, asked
// once, not tried again.
func TestAskedTooOften(t *testing.T) {
	f, c, _, _ := linked(t)
	f.mu.Lock()
	f.choices = func(method, path string, body []byte) (int, any) { return 429, codeAskedTooOften }
	f.mu.Unlock()
	if err := c.StartProgress(context.Background(), "r"); !errors.Is(err, ErrAskedTooOften) {
		t.Errorf("start: %v", err)
	}
	if n := f.count("/api/v1/choices/progress/start"); n != 1 {
		t.Errorf("asked %d times", n)
	}
}
