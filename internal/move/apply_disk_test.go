//go:build !js

package move

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyRemovesLearnerFolderOnDiskButNotThroughSymlinks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("AppData", home)
	root, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	root = filepath.Join(root, "halpwords")
	outside := t.TempDir()
	keepme := filepath.Join(outside, "keep.txt")
	os.WriteFile(keepme, []byte("mine"), 0o644)
	os.MkdirAll(filepath.Join(root, "profiles", "gone", "reports"), 0o755)
	os.WriteFile(filepath.Join(root, "profiles", "gone", "ai.json"), []byte("k"), 0o600)
	os.WriteFile(filepath.Join(root, "profiles", "gone", "reports", "r.json"), []byte("r"), 0o644)
	if err := os.Symlink(outside, filepath.Join(root, "profiles", "linked")); err != nil {
		t.Skip("no symlinks:", err)
	}
	in := &Incoming{ID: "x", Files: map[string][]byte{"profiles/new/p.json": []byte("n")}}
	if err := in.Apply(Saves); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "profiles", "gone")); !os.IsNotExist(err) {
		t.Errorf("learner folder still there: %v", err)
	}
	if b, err := os.ReadFile(keepme); err != nil || string(b) != "mine" {
		t.Errorf("a file outside the user's folder was touched: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "profiles", "new", "p.json")); err != nil {
		t.Error(err)
	}
}
