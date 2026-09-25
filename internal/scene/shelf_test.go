//go:build !js

package scene

import (
	"strings"
	"testing"

	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/words"
)

func useTempUserDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("AppData", dir)
}

func importList(t *testing.T, src string) *words.List {
	t.Helper()
	l, err := words.ParseImport(strings.NewReader(src), "import.txt")
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// loadShelf reads the shelf back from disk, as the Word Lists screen does.
func loadShelf(t *testing.T) *shelf {
	t.Helper()
	starters, err := game.StarterLists()
	if err != nil {
		t.Fatal(err)
	}
	user, errs := game.UserLists()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	return newShelf(starters, user)
}

func rowFor(s *shelf, file string) int {
	for i, r := range s.rows {
		if r.file == file {
			return i
		}
	}
	return -1
}

func TestShelfImportSaveDelete(t *testing.T) {
	useTempUserDir(t)
	s := loadShelf(t)
	starters := len(s.rows)

	// A new list, and words added to a starter list.
	imp := importList(t, "title: My Pets\nlanguage: fr\nhamster = le hamster\n")
	i := s.create(imp)
	if s.rows[i].file != "my-pets.txt" || !s.unsaved() {
		t.Fatalf("got %+v", s.rows[i])
	}
	fr := rowFor(s, "french.txt")
	before := len(s.rows[fr].starter.Entries)
	if n := s.add(fr, importList(t, "language: fr\ndog = le chien\nfrog = la grenouille\n")); n != 1 {
		t.Fatalf("added %d, want 1 (dog is already there)", n)
	}
	if len(s.rows[fr].starter.Entries) != before {
		t.Fatal("adding words changed the built-in list")
	}
	if err := s.save(); err != nil {
		t.Fatal(err)
	}
	if s.unsaved() {
		t.Fatal("still unsaved after saving")
	}

	// Everything comes back from disk, and the game uses the edited copy.
	s = loadShelf(t)
	if len(s.rows) != starters+1 {
		t.Fatalf("got %d rows, want %d", len(s.rows), starters+1)
	}
	fr = rowFor(s, "french.txt")
	if r := s.rows[fr]; !r.own || len(r.list.Entries) != before+1 {
		t.Fatalf("french.txt: own %v, %d words", r.own, len(r.list.Entries))
	}
	ctx := &game.Context{}
	if err := ctx.LoadLists(); err != nil {
		t.Fatal(err)
	}
	if got := len(ctx.ListsFor("fr")); got != 2 {
		t.Fatalf("got %d French lists, want 2", got)
	}

	// Deleting both: the new list goes, the starter comes back.
	d, r := s.deleteRows([]int{fr, rowFor(s, "my-pets.txt")})
	if d != 1 || r != 1 {
		t.Fatalf("deleted %d, reset %d", d, r)
	}
	if err := s.save(); err != nil {
		t.Fatal(err)
	}
	s = loadShelf(t)
	fr = rowFor(s, "french.txt")
	if len(s.rows) != starters || s.rows[fr].own || len(s.rows[fr].list.Entries) != before {
		t.Fatalf("after delete: %d rows, french own %v", len(s.rows), s.rows[fr].own)
	}
}

func TestShelfNewFileNamesAreUnique(t *testing.T) {
	s := newShelf(nil, nil)
	imp := importList(t, "title: French\nlanguage: fr\ndog = le chien\n")
	a, b := s.create(imp), s.create(imp)
	if s.rows[a].file != "french.txt" || s.rows[b].file != "french-2.txt" {
		t.Fatalf("got %q and %q", s.rows[a].file, s.rows[b].file)
	}
	if s.rows[a].list == imp {
		t.Fatal("create kept the imported list instead of a copy")
	}
}

func TestCleanPath(t *testing.T) {
	for in, want := range map[string]string{
		`"/tmp/my words.txt"`: "/tmp/my words.txt",
		`/tmp/my\ words.txt`:  "/tmp/my words.txt",
		" /tmp/a.txt ":        "/tmp/a.txt",
	} {
		if got := cleanPath(in); got != want {
			t.Errorf("cleanPath(%q) = %q, want %q", in, got, want)
		}
	}
}
