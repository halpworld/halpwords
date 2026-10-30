package game

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/halpworld/halpwords/internal/profile"
	"github.com/halpworld/halpwords/internal/save"
)

// After a recovery, a new failure streak is told again; and a streak that
// goes on is told only once even when polled every frame.
func TestSaveFailureNoticeAgainAfterRecovery(t *testing.T) {
	useTempDir(t)
	prof, _ := profile.Load()
	c := &Context{Profile: prof}
	dir, err := save.Current().Dir()
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "progress.json")
	fail := func() {
		os.RemoveAll(target)
		os.MkdirAll(filepath.Join(target, "x"), 0o755)
	}
	fail()
	if prof.SaveMemory() == nil {
		t.Skip("could not make the save fail here")
	}
	told := 0
	poll := func() {
		c.noticeT = 0
		c.noteSaveFailure()
		if c.noticeT > 0 {
			told++
		}
	}
	for i := 0; i < 50; i++ {
		prof.SaveMemory()
		poll()
	}
	if told != 1 {
		t.Fatalf("told %d times in one streak", told)
	}
	os.RemoveAll(target)
	if err := prof.SaveMemory(); err != nil {
		t.Fatal(err)
	}
	poll()
	if told != 1 {
		t.Fatal("told on recovery")
	}
	fail()
	prof.SaveMemory()
	poll()
	if told != 2 {
		t.Errorf("a fresh streak was told %d times in all, want 2", told)
	}
	// A nil profile or no profile must not panic.
	(&Context{}).noteSaveFailure()
}

// Pristine for the edge names: only exactly "Player <digits>" is the
// game's own.
func TestPristineEdgeNames(t *testing.T) {
	ctx, _, _ := sharedComputer(t)
	for n, want := range map[string]bool{"Player 07": true, "Player One": false, "Player 1a": false} {
		l, _ := ctx.Learners.Add("")
		l.Name = n
		if got := ctx.Pristine(l); got != want {
			t.Errorf("Pristine(%q) = %v, want %v", n, got, want)
		}
	}
}
