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
