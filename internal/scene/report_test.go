package scene

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/report"
	"github.com/halpworld/halpwords/internal/save"
)

// busyOutbox is a report outbox in the save folder whose server is
// always busy, so reports stay queued for the test to read.
func busyOutbox(t *testing.T) *report.Outbox {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	o := game.NewOutbox()
	o.Sender.Server = srv.URL
	return o
}

func TestPauseMenuHasReport(t *testing.T) {
	ctx := testContext(t)
	c := testCrawl(t, ctx)
	c.pause(ctx)
	found := false
	for _, it := range c.pauseItems() {
		found = found || it == pauseReport
	}
	if !found || !c.canChoose(pauseReport) {
		t.Fatal("no Report in the pause menu")
	}
}

func TestReportCrashOnlyWhenTicked(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	ctx.Reports = busyOutbox(t)
	if err := save.Write(game.CrashFile, []byte("Halpwords crashed.\npanic: boom\n")); err != nil {
		t.Fatal(err)
	}

	r := newReport(ctx, "ABCD-1234")
	if r.crash == "" {
		t.Fatal("crash.txt not read")
	}
	r.kind, r.text = report.KindBug, []rune("It froze")
	r.send(ctx)
	r2 := newReport(ctx, "ABCD-1234")
	r2.kind, r2.sendCrash = report.KindBug, true
	r2.send(ctx)
	if r.failed || r2.failed || r2.step != reportDone {
		t.Fatal("reports not queued")
	}

	rs, err := ctx.Reports.Queue.Pending()
	if err != nil || len(rs) != 2 {
		t.Fatalf("queued %d, %v", len(rs), err)
	}
	if rs[0].Crash != "" || rs[0].Text != "It froze" || rs[0].Seed != "ABCD-1234" || rs[0].Game.Platform == "" {
		t.Errorf("unticked: %+v", rs[0])
	}
	if !strings.Contains(rs[1].Crash, "panic: boom") {
		t.Errorf("ticked: %+v", rs[1])
	}
	// The queue file holds the crash only for the report it was ticked on.
	b, _ := save.Read(report.QueueFile)
	if strings.Count(string(b), "panic: boom") != 1 {
		t.Errorf("queue file: %s", b)
	}
}

func TestReportUpset(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	ctx.Reports = busyOutbox(t)
	r := newReport(ctx, "")
	r.kind, r.about = report.KindUpset, "picture"
	r.send(ctx)
	rs, _ := ctx.Reports.Queue.Pending()
	if len(rs) != 1 || rs[0].Kind != report.KindUpset || rs[0].About != "picture" || rs[0].Seed != "" || rs[0].Crash != "" {
		t.Fatalf("queued %+v", rs)
	}
	if linked(ctx) {
		t.Error("an unlinked game counts as linked")
	}
	// Without an outbox (it can't be saved) the player is told.
	ctx.Reports = nil
	r.send(ctx)
	if !r.failed {
		t.Error("no outbox, but the report counts as queued")
	}
}
