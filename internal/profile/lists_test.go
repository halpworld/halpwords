//go:build !js

package profile

import (
	"slices"
	"strings"
	"testing"

	"github.com/halpworld/halpwords/internal/save"
)

// The lists ticked before a run and the sent lists announced are kept
// with the settings (#89); settings from before have neither, and every
// list is ticked.
func TestListsKeptWithTheSettings(t *testing.T) {
	useTempDir(t)
	if err := save.Write(settingsFile, []byte(`{"Langs":{"fr":{"Timer":1}}}`)); err != nil {
		t.Fatal(err)
	}
	p, errs := Load()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if _, ok := p.Settings.Lists.Picked("fr"); ok || p.Settings.Lists.WasSeen("lst_1") {
		t.Fatalf("old settings have lists: %+v", p.Settings.Lists)
	}
	if err := p.SaveSettings(); err != nil {
		t.Fatal(err)
	}
	if data, _ := save.Read(settingsFile); strings.Contains(string(data), "Lists") {
		t.Errorf("no lists written as %s", data)
	}

	keys := []string{"lst_1", "file:animals.txt"}
	p.Settings.Lists.Pick("fr", keys)
	keys[0] = "changed" // Pick keeps a copy
	p.Settings.Lists.See("lst_1")
	p.Settings.Lists.See("lst_1")
	if err := p.SaveSettings(); err != nil {
		t.Fatal(err)
	}
	q, errs := Load()
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	got, ok := q.Settings.Lists.Picked("fr")
	if !ok || !slices.Equal(got, []string{"lst_1", "file:animals.txt"}) {
		t.Errorf("picked %v %v", got, ok)
	}
	if _, ok := q.Settings.Lists.Picked("la"); ok {
		t.Error("Latin has a pick")
	}
	if !q.Settings.Lists.WasSeen("lst_1") || len(q.Settings.Lists.Seen) != 1 {
		t.Errorf("seen %v", q.Settings.Lists.Seen)
	}
	// An empty pick is no pick: every list is ticked.
	q.Settings.Lists.Pick("fr", nil)
	if _, ok := q.Settings.Lists.Picked("fr"); ok {
		t.Error("an empty pick counts")
	}
}
