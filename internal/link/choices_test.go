package link

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/halpworld/halpwords/pkg/compete"
	"github.com/halpworld/halpwords/pkg/words"
)

func (f *fake) stop(on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = on
}

// TestProgressStoppedKeepsLink: while the server answers 403 not_linkable
// with reason progress_stopped (halpwords-server PLAN §25, 2026-10-12),
// the game stays linked and keeps its lists, drops what waited to be
// sent, queues nothing more (even after a restart), isn't offline, syncs
// at the usual times and picks up by itself once the progress is kept
// again.
func TestProgressStoppedKeepsLink(t *testing.T) {
	f, c, st, clk := linked(t)
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	f.stop(true)
	for range 4 {
		if err := c.Sync(context.Background()); !errors.Is(err, ErrProgressStopped) {
			t.Fatalf("sync: %v", err)
		}
	}
	s := c.Status()
	if !s.Linked || !s.Stopped || s.Suspended || s.Offline || s.Err != nil || s.Pending != 0 || s.Note != "" {
		t.Fatalf("status: %+v", s)
	}
	if c.wait() != SyncEvery {
		t.Errorf("waits %v, not the usual %v", c.wait(), SyncEvery)
	}
	if c.AccessToken() == "" || len(c.Lists()) != 1 || f.unlinked != 0 {
		t.Error("link or lists lost")
	}
	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	c.Session(Session{Start: clk.now(), Mode: "practice", Lang: "fr", Secs: 60})
	if s := c.Status(); s.Pending != 0 {
		t.Errorf("%d events queued while stopped", s.Pending)
	}
	if n := len(f.eventsOf("answers")); n != 0 {
		t.Errorf("%d answers stored while stopped", n)
	}
	c.Save()

	c2 := reopen(f, st, clk)
	if s := c2.Status(); !s.Linked || !s.Stopped {
		t.Fatalf("after a restart: %+v", s)
	}
	c2.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	if s := c2.Status(); s.Pending != 0 {
		t.Error("queued after a restart while stopped")
	}

	f.stop(false)
	if err := c2.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s := c2.Status(); s.Stopped || s.Err != nil {
		t.Fatalf("kept again: %+v", s)
	}
	c2.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	if s := c2.Status(); s.Pending != 1 {
		t.Errorf("not queuing once kept again: %+v", s)
	}
}

// TestProgressStoppedRuns: a stopped learner's finished runs are dropped
// and new ones aren't kept for the rankings.
func TestProgressStoppedRuns(t *testing.T) {
	f, c, _, clk := linked(t)
	c.mu.Lock()
	c.st.Runs = []qRun{{Code: "x", At: clk.now()}}
	c.mu.Unlock()
	f.stop(true)
	c.Sync(context.Background())
	c.Run(compete.Run{}, "")
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.st.Runs) != 0 {
		t.Errorf("runs kept: %+v", c.st.Runs)
	}
}

// TestNotLinkableReason: only reason progress_stopped is a stopped
// learner; not_linkable alone still is ErrNotLinkable.
func TestNotLinkableReason(t *testing.T) {
	e := errorFrom(403, nil, []byte(`{"error": {"code": "not_linkable", "reason": "progress_stopped", "message": "m"}}`))
	if e.Reason != reasonProgressStopped || !errors.Is(notLinkable(e), ErrProgressStopped) || !stopsSync(context.Background(), notLinkable(e)) {
		t.Errorf("progress_stopped: %+v", e)
	}
	if err := notLinkable(&Error{Status: 403, Code: codeNotLinkable}); !errors.Is(err, ErrNotLinkable) {
		t.Errorf("not_linkable: %v", err)
	}
	if Explain(ErrProgressStopped) != ErrProgressStopped.Error() {
		t.Error("not explained")
	}
}

// choicesServer is My choices on the fake server, for a 16-year-old.
type choicesServer struct {
	stopped, waiting, older bool
	seen                    string
	posts                   []string
	bodies                  []map[string]any
}

func (s *choicesServer) serve(f *fake) func(method, path string, body []byte) (int, any) {
	return func(method, path string, body []byte) (int, any) {
		var req map[string]any
		json.Unmarshal(body, &req)
		if method == "POST" {
			s.posts = append(s.posts, path)
			s.bodies = append(s.bodies, req)
		}
		switch path {
		case "/api/v1/choices":
			p := map[string]any{"can_stop": !s.stopped, "stopped": s.stopped, "waiting": s.waiting}
			if s.stopped && !s.waiting {
				p["restart"] = map[string]any{"ref": "teen-progress/1/en/r", "title": "Keep it", "body": []string{"b"},
					"label": "Keep my progress", "hint": "h"}
			}
			return 200, map[string]any{"family": true,
				"child_notice": map[string]any{"ref": "child-link/1/en/c", "title": "Who can see", "body": []string{"Your parent."}},
				"older_notice": s.older,
				"visibility": map[string]any{"ref": "teen-visibility/1/en/v", "title": "t", "body": []string{"b"},
					"asks": []any{map[string]any{"purpose": "rankings", "agreed": false, "label": "Rankings", "hint": "h"}}},
				"progress": p}
		case "/api/v1/choices/notice":
			if req["notice_ref"] != "child-link/1/en/c" {
				return 409, codeNoticeChanged
			}
			s.seen = req["notice_ref"].(string)
		case "/api/v1/choices/older":
			s.older = false
		case "/api/v1/choices/agreement":
			if req["purpose"] != "rankings" {
				return 409, codeNotAsked
			}
		case "/api/v1/choices/progress/stop":
			if s.stopped {
				return 409, codeNotOwn
			}
			s.stopped = true
			f.stopped = true
		case "/api/v1/choices/progress/start":
			if !s.stopped || s.waiting || req["notice_ref"] != "teen-progress/1/en/r" {
				return 409, codeNoticeChanged
			}
			s.waiting = true
		case "/api/v1/choices/progress/cancel":
			if !s.waiting {
				return 409, codeNotOwn
			}
			s.waiting = false
		default:
			return 404, "not_found"
		}
		return 204, nil
	}
}

// TestChoices walks My choices against the fake server: the child-link
// notice is recorded once, the 16-17 notice marked, an agreement given
// and taken back, the progress stopped (the game stays linked and
// queues nothing), asked for again, and the request cancelled, all
// while the other routes are refused and the tokens are refreshed.
func TestChoices(t *testing.T) {
	f, c, st, clk := linked(t)
	cs := &choicesServer{older: true}
	f.mu.Lock()
	f.choices = cs.serve(f)
	f.mu.Unlock()
	ctx := context.Background()

	if !c.ChildNoticeDue() {
		t.Fatal("child notice not due after linking")
	}
	ch, err := c.Choices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !ch.Family || ch.ChildNotice.Title != "Who can see" || !ch.OlderNotice || ch.Visibility == nil ||
		len(ch.Visibility.Asks) != 1 || ch.Progress == nil || !ch.Progress.CanStop {
		t.Fatalf("choices: %+v", ch)
	}
	if err := c.SawNotice(ctx, "child-link/0/en/old"); !errors.Is(err, ErrNoticeChanged) {
		t.Errorf("stale notice: %v", err)
	}
	if err := c.SawNotice(ctx, ch.ChildNotice.Ref); err != nil {
		t.Fatal(err)
	}
	if c.ChildNoticeDue() || reopen(f, st, clk).ChildNoticeDue() {
		t.Error("child notice due again")
	}
	if err := c.MarkOlder(ctx); err != nil || cs.older {
		t.Errorf("older: %v", err)
	}
	if err := c.SetAgreement(ctx, "rankings", true, ch.Visibility.Ref); err != nil {
		t.Fatal(err)
	}
	if err := c.SetAgreement(ctx, "rankings", false, ch.Visibility.Ref); err != nil {
		t.Fatal(err)
	}
	if got := cs.bodies[len(cs.bodies)-2]; got["notice_ref"] != ch.Visibility.Ref || got["agree"] != true {
		t.Errorf("agreed with %v", got)
	}
	if _, ok := cs.bodies[len(cs.bodies)-1]["notice_ref"]; ok {
		t.Error("saying no sent a notice")
	}
	if err := c.SetAgreement(ctx, "friend_groups", true, ch.Visibility.Ref); !errors.Is(err, ErrNotOwn) {
		t.Errorf("not asked: %v", err)
	}

	c.Answer("fr", dog, "practice", words.Answer{Tier: words.Perfect})
	if err := c.StopProgress(ctx); err != nil {
		t.Fatal(err)
	}
	if got := cs.bodies[len(cs.bodies)-1]; got["confirm"] != true {
		t.Errorf("stopped with %v", got)
	}
	if s := c.Status(); !s.Linked || !s.Stopped || s.Pending != 0 {
		t.Fatalf("after stopping: %+v", s)
	}
	if err := c.StopProgress(ctx); !errors.Is(err, ErrNotOwn) {
		t.Errorf("stopped twice: %v", err)
	}
	if err := c.Sync(ctx); !errors.Is(err, ErrProgressStopped) {
		t.Errorf("sync: %v", err)
	}

	// The access token runs out: My choices still refreshes it.
	c.mu.Lock()
	c.st.AccessExp = time.Time{}
	c.mu.Unlock()
	if ch, err = c.Choices(ctx); err != nil || !ch.Progress.Stopped || ch.Progress.Restart == nil {
		t.Fatalf("while stopped: %+v %v", ch, err)
	}
	if err := c.StartProgress(ctx, ch.Progress.Restart.Ref); err != nil {
		t.Fatal(err)
	}
	if ch, err = c.Choices(ctx); err != nil || !ch.Progress.Waiting || ch.Progress.Restart != nil {
		t.Fatalf("waiting: %+v %v", ch.Progress, err)
	}
	if err := c.CancelProgress(ctx); err != nil || cs.waiting {
		t.Fatalf("cancel: %v", err)
	}
	if s := c.Status(); !s.Linked || !s.Stopped || f.unlinked != 0 {
		t.Errorf("at the end: %+v", s)
	}
}

// TestChoicesOldServer: a server without My choices answers 404, which
// is ErrNoChoices; nothing else changes.
func TestChoicesOldServer(t *testing.T) {
	_, c, _, _ := linked(t)
	if _, err := c.Choices(context.Background()); !errors.Is(err, ErrNoChoices) {
		t.Errorf("choices: %v", err)
	}
	if err := c.SawNotice(context.Background(), "child-link/1/en/c"); !errors.Is(err, ErrNoChoices) {
		t.Errorf("notice: %v", err)
	}
	if s := c.Status(); !s.Linked || s.Stopped || !c.ChildNoticeDue() {
		t.Errorf("status: %+v", s)
	}
}

// TestChoicesNotLinked: My choices of a game that isn't linked.
func TestChoicesNotLinked(t *testing.T) {
	_, c, _, _ := setup(t)
	if _, err := c.Choices(context.Background()); !errors.Is(err, ErrNotLinked) {
		t.Errorf("choices: %v", err)
	}
	if c.ChildNoticeDue() {
		t.Error("due when not linked")
	}
}
