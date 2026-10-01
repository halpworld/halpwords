package game

import (
	"errors"
	"testing"
	"time"

	"github.com/halpworld/halpwords/internal/profile"
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

// sharedComputer makes a computer with a learner playing and a learner
// who signed in at school, with a lock, and played more recently.
func sharedComputer(t *testing.T) (ctx *Context, playing, locked *profile.Learner) {
	useTempDir(t)
	ctx = &Context{Sound: &Sound{Muted: true}}
	ctx.openLearners()
	if ctx.Learners == nil {
		t.Fatal("no learners")
	}
	ctx.openLink()
	t.Cleanup(func() { ctx.Link.Close() })
	ctx.loadProfile()
	playing = ctx.Learner()
	var err error
	if locked, err = ctx.AddLearner("Brian"); err != nil {
		t.Fatal(err)
	}
	if locked.Lock, err = profile.NewLock(profile.LockCard, "ABCD"); err != nil {
		t.Fatal(err)
	}
	locked.LastUsed = time.Now().Add(time.Hour) // the most recent
	ctx.Learners.Save()
	if err := ctx.SwitchLearner(playing.ID); err != nil {
		t.Fatal(err)
	}
	return ctx, playing, locked
}

// Nobody can switch to a learner with a lock without opening it.
func TestSwitchNeedsLock(t *testing.T) {
	ctx, _, locked := sharedComputer(t)
	if err := ctx.SwitchLearner(locked.ID); !errors.Is(err, ErrLearnerLocked) {
		t.Fatalf("SwitchLearner(locked) = %v, want ErrLearnerLocked", err)
	}
	if ok, err := ctx.UnlockLearner(locked.ID, "WRONG"); ok || err != nil {
		t.Fatalf("wrong secret: %v, %v", ok, err)
	}
	if ctx.Learner() == locked {
		t.Fatal("wrong secret switched")
	}
	if ok, err := ctx.UnlockLearner(locked.ID, "ABCD"); !ok || err != nil || ctx.Learner() != locked {
		t.Fatalf("right secret: %v, %v, playing %v", ok, err, ctx.Learner())
	}
}

// Removing the learner playing never lands on a learner with a lock (#38).
func TestRemoveSkipsLocked(t *testing.T) {
	ctx, _, locked := sharedComputer(t)
	// A pupil signs in as the second learner ... and a third is playing.
	third, err := ctx.AddLearner("Cara")
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.RemoveLearner(third.ID); err != nil {
		t.Fatal(err)
	}
	cur := ctx.Learner()
	if cur == nil || cur.Lock != nil || cur.ID == locked.ID {
		t.Fatalf("playing %+v after removal", cur)
	}
}

// With only locked learners left, removing the one playing gives a new,
// empty learner.
func TestRemoveOnlyLockedLeft(t *testing.T) {
	ctx, first, locked := sharedComputer(t)
	first.Lock, _ = profile.NewLock(profile.LockCard, "WXYZ")
	ctx.Learners.Save()
	third, err := ctx.AddLearner("Cara") // no lock
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.RemoveLearner(third.ID); err != nil {
		t.Fatal(err)
	}
	cur := ctx.Learner()
	if cur == nil || cur.Lock != nil || cur.ID == first.ID || cur.ID == locked.ID || cur.LearnerID != "" {
		t.Fatalf("playing %+v", cur)
	}
	if len(ctx.Learners.List) != 3 {
		t.Errorf("%d learners, want 3", len(ctx.Learners.List))
	}
}

// Signing out when the progress stays leaves a learner with a lock: the
// game must not stay on them, unlocked (#38).
func TestSignOutKeepLeavesLocked(t *testing.T) {
	ctx, first, locked := sharedComputer(t)
	if _, err := ctx.UnlockLearner(locked.ID, "ABCD"); err != nil || ctx.Learner() != locked {
		t.Fatalf("unlock: %v", err)
	}
	done := ctx.SignOut()
	for !done() {
	}
	cur := ctx.Learner()
	if cur == nil || cur.Lock != nil {
		t.Fatalf("playing %+v after signing out", cur)
	}
	if cur.ID != first.ID {
		t.Errorf("playing %q, want the unlocked learner %q", cur.ID, first.ID)
	}
	if ctx.Learners.Find(locked.ID) == nil {
		t.Error("the progress that stays is gone")
	}
}

// Giving up adding a learner goes back to the one before, even with a lock.
func TestCancelAddLearner(t *testing.T) {
	ctx, _, locked := sharedComputer(t)
	if _, err := ctx.UnlockLearner(locked.ID, "ABCD"); err != nil {
		t.Fatal(err)
	}
	added, err := ctx.AddLearner("")
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.CancelAddLearner(); err != nil {
		t.Fatal(err)
	}
	if ctx.Learner() != locked || ctx.Learners.Find(added.ID) != nil {
		t.Errorf("playing %v, new learner still listed: %v", ctx.Learner(), ctx.Learners.Find(added.ID) != nil)
	}
}
