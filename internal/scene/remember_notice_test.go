package scene

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/halpworld/halpwords/internal/profile"
	"github.com/halpworld/halpwords/internal/save"
)

// run.remember records a failed memory save, and clears the "told" flag
// again once a save works, so a later failure is reported afresh.
func TestRememberTracksFailureStreak(t *testing.T) {
	useTempDir(t)
	t.Cleanup(func() { save.Use(save.Root) })
	ctx := testContext(t)
	prof, _ := profile.Load()
	ctx.Profile = prof
	r, _ := testRun(t, ctx)

	r.remember()
	if r.memFailed {
		t.Fatal("a healthy save counted as failed")
	}
	dir, err := save.Current().Dir()
	if err != nil {
		t.Fatal(err)
	}
	// A directory where progress.json should be makes the rename fail.
	target := filepath.Join(dir, "progress.json")
	os.Remove(target)
	if err := os.MkdirAll(filepath.Join(target, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	r.remember()
	if !r.memFailed {
		t.Skip("could not make the save fail on this platform")
	}
	r.memNoticed = true // the player was told
	r.remember()
	if !r.memFailed || !r.memNoticed {
		t.Error("told flag cleared during a failure streak")
	}
	os.RemoveAll(target)
	r.remember()
	if r.memFailed || r.memNoticed {
		t.Errorf("recovery did not reset: failed=%v noticed=%v", r.memFailed, r.memNoticed)
	}
}
