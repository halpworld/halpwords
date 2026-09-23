//go:build !js

package save

import (
	"errors"
	"io/fs"
	"testing"
)

func TestReadWriteRemove(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("AppData", dir)

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
