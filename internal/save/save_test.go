//go:build !js

package save

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
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
