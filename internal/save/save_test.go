//go:build !js

package save

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func useTempDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("AppData", dir)
}

func TestReadWriteRemove(t *testing.T) {
	useTempDir(t)

	if _, err := Read("test.json"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("read before write: got %v, want not exist", err)
	}
	for _, s := range []string{"first", "second"} {
		if err := Write("test.json", []byte(s)); err != nil {
			t.Fatal(err)
		}
		got, err := Read("test.json")
		if err != nil || string(got) != s {
			t.Fatalf("got %q, %v; want %q", got, err, s)
		}
	}
	if err := Remove("test.json"); err != nil {
		t.Fatal(err)
	}
	if _, err := Read("test.json"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("read after remove: got %v, want not exist", err)
	}
	if err := Remove("test.json"); err != nil {
		t.Fatalf("removing twice: %v", err)
	}
}

func TestFolders(t *testing.T) {
	useTempDir(t)

	if names, err := List("words"); err != nil || len(names) != 0 {
		t.Fatalf("empty folder: got %v, %v", names, err)
	}
	for _, n := range []string{"words/b.txt", "words/a.txt", "other.json"} {
		if err := Write(n, []byte(n)); err != nil {
			t.Fatal(err)
		}
	}
	names, err := List("words")
	if err != nil || len(names) != 2 || names[0] != "a.txt" || names[1] != "b.txt" {
		t.Fatalf("got %v, %v; want [a.txt b.txt]", names, err)
	}
	if got, err := Read("words/b.txt"); err != nil || string(got) != "words/b.txt" {
		t.Fatalf("read: got %q, %v", got, err)
	}
	if err := Remove("words/a.txt"); err != nil {
		t.Fatal(err)
	}
	if names, _ := List("words"); len(names) != 1 {
		t.Fatalf("after remove: got %v", names)
	}
}

func TestWritePrivate(t *testing.T) {
	useTempDir(t)
	if err := WritePrivate("secret.json", []byte("key")); err != nil {
		t.Fatal(err)
	}
	dir, _ := Dir()
	fi, err := os.Stat(filepath.Join(dir, "secret.json"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v, want 0600", fi.Mode().Perm())
	}
}

// In memory, files start empty, and nothing reaches the user's folder.
func TestUseMemory(t *testing.T) {
	useTempDir(t)
	if err := Write("hero.json", []byte("on disk")); err != nil {
		t.Fatal(err)
	}
	UseMemory()
	defer func() { memMu.Lock(); memory = nil; memMu.Unlock() }()
	if !InMemory() {
		t.Fatal("not in memory")
	}
	if _, err := Dir(); err == nil {
		t.Error("a folder while in memory")
	}
	if _, err := Read("hero.json"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("read the folder's file: %v", err)
	}
	for _, n := range []string{"quests/b.hwquest", "quests/a.hwquest", "hero.json", "words/x.txt"} {
		if err := Write(n, []byte(n)); err != nil {
			t.Fatal(err)
		}
	}
	if err := WritePrivate("ai.json", []byte("key")); err != nil {
		t.Fatal(err)
	}
	if got, err := Read("hero.json"); err != nil || string(got) != "hero.json" {
		t.Errorf("read %q, %v", got, err)
	}
	if names, _ := List("quests/"); len(names) != 2 || names[0] != "a.hwquest" || names[1] != "b.hwquest" {
		t.Errorf("listed %q", names)
	}
	if err := Remove("hero.json"); err != nil {
		t.Fatal(err)
	}
	if _, err := Read("hero.json"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("read a removed file: %v", err)
	}
	memMu.Lock()
	memory = nil
	memMu.Unlock()
	if got, err := Read("hero.json"); err != nil || string(got) != "on disk" {
		t.Errorf("the folder's file is now %q, %v", got, err)
	}
	if _, err := Read("ai.json"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the key reached the folder: %v", err)
	}
}

func TestAll(t *testing.T) {
	useTempDir(t)
	if names, err := All(); err != nil || len(names) != 0 {
		t.Fatalf("empty folder: got %v, %v", names, err)
	}
	for _, n := range []string{"b.json", "words/x.txt", "a.json", "ai/bank-fr.json"} {
		if err := Write(n, []byte(n)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := All()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.json", "ai/bank-fr.json", "b.json", "words/x.txt"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestModTime(t *testing.T) {
	useTempDir(t)
	if !ModTime("words/a.txt").IsZero() {
		t.Fatal("a missing file has a time")
	}
	if err := Write("words/a.txt", []byte("x")); err != nil {
		t.Fatal(err)
	}
	old := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	dir, _ := Dir()
	if err := os.Chtimes(filepath.Join(dir, "words", "a.txt"), old, old); err != nil {
		t.Fatal(err)
	}
	if got := ModTime("words/a.txt"); !got.Equal(old) {
		t.Errorf("mod time %v, want %v", got, old)
	}
}

func TestRemoveAllDoesNotFollowSymlinks(t *testing.T) {
	useTempDir(t)
	dir, _ := Root.Dir()
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "keep.txt"), []byte("mine"), 0o644)
	os.MkdirAll(filepath.Join(dir, "profiles"), 0o755)
	if err := os.Symlink(outside, filepath.Join(dir, "profiles", "x")); err != nil {
		t.Skip("no symlinks:", err)
	}
	if err := Folder("profiles/x").RemoveAll(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outside, "keep.txt")); err != nil {
		t.Errorf("file outside was removed: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "profiles", "x")); !os.IsNotExist(err) {
		t.Errorf("link still there: %v", err)
	}
	// profiles itself is a link out: refused.
	os.RemoveAll(filepath.Join(dir, "profiles"))
	os.Symlink(outside, filepath.Join(dir, "profiles"))
	if err := Folder("profiles/y").RemoveAll(); err != nil {
		t.Logf("refused: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "keep.txt")); err != nil {
		t.Errorf("file outside was removed: %v", err)
	}
}

func TestQuarantineKeepsCopiesAndCapsThem(t *testing.T) {
	useTempDir(t)
	Write("memory.json.bad", []byte("legacy"))
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	var names []string
	for i := 0; i < 5; i++ {
		// Two in the same second get different names.
		n, err := Root.quarantine("memory.json", []byte{byte('a' + i)}, at.Add(time.Duration(i/2)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
	}
	for i, n := range names {
		for _, m := range names[i+1:] {
			if n == m {
				t.Fatalf("name %q used twice", n)
			}
		}
	}
	left, _ := List("")
	if len(left) != KeepBad {
		t.Fatalf("kept %v, want %d", left, KeepBad)
	}
	// The newest are the ones kept: c, d, e.
	for i, want := range []string{"c", "d", "e"} {
		var got string
		for _, n := range names {
			if d, err := Read(n); err == nil && string(d) == want {
				got = n
			}
		}
		if got == "" {
			t.Errorf("copy %d (%s) was removed, left %v", i, want, left)
		}
	}
	if _, err := Read("memory.json.bad"); err == nil {
		t.Error("the oldest, legacy copy was not removed")
	}
}

func TestQuarantineInAFolder(t *testing.T) {
	useTempDir(t)
	f := Folder("profiles/x")
	if _, err := f.Quarantine("memory.json", []byte("bad")); err != nil {
		t.Fatal(err)
	}
	if names, _ := f.List(""); len(names) != 1 || !strings.HasSuffix(names[0], ".bad") {
		t.Fatalf("names %v", names)
	}
	if names, _ := Root.List(""); len(names) != 0 {
		t.Fatalf("root has %v", names)
	}
}

func TestWriteLeavesNoTempFiles(t *testing.T) {
	useTempDir(t)
	for i := 0; i < 3; i++ {
		if err := Write("a/b.json", []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if names, _ := List("a"); len(names) != 1 || names[0] != "b.json" {
		t.Fatalf("names %v", names)
	}
}
