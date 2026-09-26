package game

import (
	"testing"

	"github.com/halpworld/halpwords/internal/save"
)

// useTempDir points the save folder at a fresh temporary folder.
func useTempDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("AppData", dir)
	t.Cleanup(func() { save.Use(save.Root) })
}

// Switching learners loads each one's own profile, and removing one
// deletes theirs and switches back (W2.5).
func TestSwitchLearners(t *testing.T) {
	useTempDir(t)
	ctx := &Context{Sound: &Sound{Muted: true}}
	ctx.openLearners()
	if ctx.Learners == nil {
		t.Fatal("no learners")
	}
	ctx.openLink()
	defer func() { ctx.Link.Close() }()
	ctx.loadProfile()
	first := ctx.Learner()
	ctx.Profile.Name = "Aoife"
	if err := ctx.Profile.SaveFame(); err != nil {
		t.Fatal(err)
	}

	second, err := ctx.AddLearner("Brian")
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Learner() != second || save.Current() != second.Folder() {
		t.Fatalf("playing %v in %q", ctx.Learner(), save.Current())
	}
	if ctx.Profile.Name != "" {
		t.Errorf("new learner has %q's name", ctx.Profile.Name)
	}
	ctx.Profile.Name = "Brian"
	ctx.Profile.SaveFame()

	if err := ctx.SwitchLearner(first.ID); err != nil {
		t.Fatal(err)
	}
	if ctx.Profile.Name != "Aoife" {
		t.Errorf("back to the first learner: name %q", ctx.Profile.Name)
	}

	if err := ctx.SwitchLearner(second.ID); err != nil {
		t.Fatal(err)
	}
	if err := ctx.RemoveLearner(second.ID); err != nil {
		t.Fatal(err)
	}
	if ctx.Learner() != first || ctx.Profile.Name != "Aoife" {
		t.Errorf("after removing, playing %v named %q", ctx.Learner(), ctx.Profile.Name)
	}
	if names, _ := second.Folder().All(); len(names) > 0 {
		t.Errorf("removed learner's files are left: %v", names)
	}
	if len(ctx.Learners.List) != 1 {
		t.Errorf("%d learners, want 1", len(ctx.Learners.List))
	}
}
