package link

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/halpworld/halpwords/pkg/compete"
	"github.com/halpworld/halpwords/pkg/words"
)

// totalsSum is the answers the fake server holds in totals events.
func totalsSum(f *fake) (n int) {
	for _, e := range f.eventsOf("totals") {
		n += int(e.Raw["answers"].(float64))
	}
	return n
}

// A batch the server stored but whose reply was lost, then more answers
// the same day, then a restart: the server ends with every answer once.
func TestQA3LostReplyThenMoreAnswersThenRestart(t *testing.T) {
	f, c, st, clk := linked(t)
	var lose atomic.Bool
	lose.Store(true)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/events" && lose.Load() {
			f.srv.Config.Handler.ServeHTTP(httptest.NewRecorder(), r) // stored, reply dropped
			writeErr(w, http.StatusServiceUnavailable, "internal")
			return
		}
		f.srv.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(proxy.Close)
	c.server = proxy.URL
	for range 2 {
		c.Answer("fr", own, "practice", words.Answer{Tier: words.Perfect})
	}
	if err := c.Sync(context.Background()); err == nil {
		t.Fatal("the upload didn't fail")
	}
	if got := totalsSum(f); got != 2 {
		t.Fatalf("server holds %d before the restart, want 2 (the fake didn't store it)", got)
	}
	for range 3 {
		c.Answer("fr", own, "practice", words.Answer{Tier: words.Correct})
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	lose.Store(false)
	c2 := Open(Options{Store: st, Server: proxy.URL, OwnDir: "words", Now: clk.now})
	c2.sleep = func(time.Duration) {}
	if err := c2.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := totalsSum(f); got != 5 {
		t.Errorf("totals on the server count %d answers, want 5", got)
	}
}

// The Sent mark is kept when the events are parked and the same learner
// links again: an answer added then goes into a new event, not the sent one.
func TestQA3SentFlagSurvivesParkAndRelink(t *testing.T) {
	f, c, st, clk := linked(t)
	c.Answer("fr", own, "practice", words.Answer{Tier: words.Perfect})
	f.failNext("/api/v1/events", 503, 503, 503)
	if err := c.Sync(context.Background()); err == nil {
		t.Fatal("the upload didn't fail")
	}
	loseLink(t, f, c)
	c.Close()
	c2 := reopen(f, st, clk)
	f.mu.Lock()
	f.code = "QRST-VWXY"
	f.mu.Unlock()
	if err := c2.LinkNow(context.Background(), "QRSTVWXY"); err != nil {
		t.Fatal(err)
	}
	// The parked events come back at the first sync; make its send fail.
	f.failNext("/api/v1/events", 503, 503, 503)
	if err := c2.Sync(context.Background()); err == nil {
		t.Fatal("the upload didn't fail")
	}
	c2.Answer("fr", own, "practice", words.Answer{Tier: words.Perfect})
	c2.mu.Lock()
	var sent, open int
	for _, tt := range c2.q.Totals {
		if tt.Sent {
			sent++
			if tt.Answers != 1 {
				t.Errorf("a sent event changed: %+v", tt)
			}
		} else {
			open++
		}
	}
	c2.mu.Unlock()
	if sent != 1 || open != 1 {
		t.Errorf("after relink: %d sent, %d open totals events, want 1 and 1", sent, open)
	}
	if err := c2.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := totalsSum(f); got != 2 {
		t.Errorf("server holds %d answers in totals, want 2", got)
	}
}

// A 500 on the lists doesn't keep a finished ranked run from being sent.
func TestQA3ListsErrorStillUploadsRun(t *testing.T) {
	f, c, _, _ := linked(t)
	f.mu.Lock()
	f.boards = []any{}
	f.mu.Unlock()
	c.Run(compete.Run{Share: compete.Share{Lang: "fr", Seed: 7, Floor: 2, Score: 12}, Secs: 100}, "0123456789abcdef")
	f.failNext("/api/v1/lists", 500, 500, 500)
	err := c.Sync(context.Background())
	var e *Error
	if !errors.As(err, &e) || e.Status != 500 {
		t.Fatalf("sync: %v", err)
	}
	f.mu.Lock()
	n := len(f.runs)
	f.mu.Unlock()
	if n != 1 {
		t.Errorf("%d runs reached the server, want 1", n)
	}
	c.mu.Lock()
	left := len(c.st.Runs)
	c.mu.Unlock()
	if left != 0 {
		t.Errorf("%d runs still queued", left)
	}
}

// A 401 that the refresh can't cure, on a stage after the first, stops
// every stage after it (and runs are kept, not dropped).
func TestQA3UnauthorizedInLaterStageStopsTheRest(t *testing.T) {
	f, c, _, _ := linked(t)
	f.mu.Lock()
	f.boards = []any{}
	f.refresh = "" // refresh no longer works
	f.mu.Unlock()
	c.Run(compete.Run{Share: compete.Share{Lang: "fr", Seed: 7, Floor: 2, Score: 12}, Secs: 100}, "0123456789abcdef")
	f.failNext("/api/v1/assignments", 401)
	mem, ranks, runs := f.count("/api/v1/memory"), f.count("/api/v1/ranks"), f.count("/api/v1/runs")
	if err := c.Sync(context.Background()); !errors.Is(err, ErrUnlinked) {
		t.Fatalf("sync: %v", err)
	}
	if f.count("/api/v1/memory") != mem || f.count("/api/v1/ranks") != ranks || f.count("/api/v1/runs") != runs {
		t.Error("stages went on after a 401")
	}
}

// pronounced makes the fake offer pronunciation, answering packs with h.
func pronounced(f *fake, h func(id, version string) (int, []byte)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.me["pronunciation"] = map[string]any{"available": true}
	f.audio = h
}

// An unlink while a pack is downloading leaves nothing of it in the
// learner's folder or the state.
func TestQA3UnlinkDuringPackDownloadWritesNothing(t *testing.T) {
	f, c, st, _ := linked(t)
	started, release := make(chan struct{}, 1), make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	pronounced(f, func(id, version string) (int, []byte) {
		started <- struct{}{}
		<-release
		return 200, animalsPack(t, 3)
	})
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the pack was never asked for")
	}
	c.Unlink()
	close(release)
	waitAudio(t, c)
	for name := range st.files {
		if strings.HasPrefix(name, "assigned/") {
			t.Errorf("%s written after the unlink", name)
		}
	}
	if len(c.Lists()) != 0 {
		t.Errorf("lists after unlink: %v", c.Lists())
	}
	if _, ok := c.Pronunciation("fr", dog); ok {
		t.Error("a pack of the unlinked learner is used")
	}
}

// Close with a pack still downloading returns at once and the download
// goroutine ends.
func TestQA3CloseDuringPackDownload(t *testing.T) {
	f, c, _, _ := linked(t)
	started, release := make(chan struct{}, 1), make(chan struct{})
	t.Cleanup(func() { close(release) })
	pronounced(f, func(id, version string) (int, []byte) {
		started <- struct{}{}
		<-release
		return 200, animalsPack(t, 3)
	})
	if err := c.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the pack was never asked for")
	}
	t0 := time.Now()
	c.Close()
	if d := time.Since(t0); d > time.Second {
		t.Errorf("Close took %v with a pack downloading", d)
	}
	done := make(chan struct{})
	go func() { c.bg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the download goroutine is still running after Close")
	}
	c.mu.Lock()
	busy := c.audioRunning
	c.mu.Unlock()
	if busy {
		t.Error("audioRunning still set")
	}
}

func TestQA3PackBackoffSteps(t *testing.T) {
	for n, want := range map[int]time.Duration{0: 0, 1: 0, 2: 10 * time.Minute, 3: 20 * time.Minute,
		4: 40 * time.Minute, 5: 80 * time.Minute, 6: 2 * time.Hour, 7: 2 * time.Hour, 50: 2 * time.Hour} {
		if got := packBackoff(n); got != want {
			t.Errorf("packBackoff(%d) = %v, want %v", n, got, want)
		}
	}
}

// With an injected clock: a failing pack is asked for again exactly when
// its wait is over, and the wait grows 10 min, 20 min, 40 min.
func TestQA3PackBackoffTiming(t *testing.T) {
	f, c, _, clk := linked(t)
	asked := 0
	pronounced(f, func(id, version string) (int, []byte) { asked++; return 500, nil })
	sync := func() {
		t.Helper()
		if err := c.Sync(context.Background()); err != nil {
			t.Fatal(err)
		}
		waitAudio(t, c)
	}
	sync() // failure 1: no wait
	sync() // failure 2: 10 minutes
	if asked != 2 {
		t.Fatalf("asked %d, want 2", asked)
	}
	for i, wait := range []time.Duration{10 * time.Minute, 20 * time.Minute, 40 * time.Minute} {
		clk.add(wait - time.Second)
		sync()
		if asked != 2+i {
			t.Fatalf("step %d: asked %d before the wait was over", i, asked)
		}
		clk.add(time.Second)
		sync()
		if asked != 3+i {
			t.Fatalf("step %d: asked %d after the wait, want %d", i, asked, 3+i)
		}
	}
}
