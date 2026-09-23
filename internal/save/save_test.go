//go:build !js

package save

import (
	"errors"
	"io/fs"
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
