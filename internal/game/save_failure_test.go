package game

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/halpworld/halpwords/internal/profile"
	"github.com/halpworld/halpwords/internal/save"
)

// A word memory that can't be written is reported once, from any scene, for
// the per-answer write and the durable one; it is reported again after a
// recovery.
func TestSaveFailureNotice(t *testing.T) {
	useTempDir(t)
	prof, _ := profile.Load()
	c := &Context{Profile: prof}
	dir, err := save.Current().Dir()
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "progress.json")
	os.Remove(target)
	if err := os.MkdirAll(filepath.Join(target, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if prof.SaveMemory() == nil {
		t.Skip("could not make the save fail on this platform")
	}
	c.noteSaveFailure()
	if c.noticeT == 0 {
		t.Fatal("no notice for a failed save")
	}
	c.noticeT = 0
	prof.SaveMemory()
	c.noteSaveFailure()
	if c.noticeT != 0 {
		t.Error("told again for the same failure")
	}
	os.RemoveAll(target)
	if err := prof.SaveMemory(); err != nil {
		t.Fatal(err)
	}
	// Now the durable write fails.
	os.MkdirAll(filepath.Join(target, "x"), 0o755)
	prof.SaveMemory() // fails, marks unsynced
	c.noteSaveFailure()
	c.noticeT = 0
	os.RemoveAll(target)
	prof.SaveMemory()
	os.RemoveAll(target)
	os.MkdirAll(filepath.Join(target, "x"), 0o755)
	if prof.SaveMemoryDurable() == nil {
		t.Skip("durable write did not fail")
	}
	c.noteSaveFailure()
	if c.noticeT == 0 {
		t.Error("no notice for a failed durable save")
	}
}
