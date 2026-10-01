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

// The lock counts wrong tries across calls: after MaxTries the right
// secret is refused too, and nothing switches (#38).
func TestUnlockLearnerLockout(t *testing.T) {
	ctx, first, locked := sharedComputer(t)
	for i := 1; i < profile.MaxTries; i++ {
		if ok, err := ctx.UnlockLearner(locked.ID, "WRONG"); ok || err != nil {
			t.Fatalf("try %d: %v, %v", i, ok, err)
		}
	}
	if ok, err := ctx.UnlockLearner(locked.ID, "WRONG"); ok || !errors.Is(err, profile.ErrLocked) {
		t.Fatalf("last try: %v, %v, want ErrLocked", ok, err)
	}
	if ok, err := ctx.UnlockLearner(locked.ID, "ABCD"); ok || !errors.Is(err, profile.ErrLocked) {
		t.Fatalf("right secret while locked out: %v, %v", ok, err)
	}
	if ctx.Learner() != first {
		t.Fatalf("playing %v, want to stay with %v", ctx.Learner(), first)
	}
	// Unlocking a learner who has no lock switches to them.
	free, err := ctx.Learners.Add("Free")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := ctx.UnlockLearner(free.ID, ""); !ok || err != nil || ctx.Learner() != free {
		t.Fatalf("learner without a lock: %v, %v", ok, err)
	}
	if ok, _ := ctx.UnlockLearner("nobody", "x"); ok {
		t.Error("unlocked a learner who doesn't exist")
	}
}

// The learner playing can "switch" to themselves, lock or not: they
// opened it already.
func TestSwitchToSelfWithLock(t *testing.T) {
	ctx, _, locked := sharedComputer(t)
	if _, err := ctx.UnlockLearner(locked.ID, "ABCD"); err != nil {
		t.Fatal(err)
	}
	if err := ctx.SwitchLearner(locked.ID); err != nil {
		t.Fatalf("SwitchLearner(self) = %v", err)
	}
}

// A new learner without a name is "Player N", the first free N.
func TestAddLearnerNameFallback(t *testing.T) {
	ctx, _, _ := sharedComputer(t) // two learners
	l, err := ctx.AddLearner("")
	if err != nil {
		t.Fatal(err)
	}
	if l.Name != "Player 3" {
		t.Errorf("name %q, want Player 3", l.Name)
	}
	if err := ctx.RemoveLearner(ctx.Learners.List[0].ID); err != nil {
		t.Fatal(err)
	}
	if l2, _ := ctx.AddLearner(""); l2.Name == "" || l2.Name == l.Name {
		t.Errorf("second unnamed learner is %q, same as the first", l2.Name)
	}
}

// Without a learner added before, cancelling does nothing: the one
// playing is not removed, and a second cancel does nothing either.
func TestCancelAddLearnerNothingToCancel(t *testing.T) {
	ctx, first, _ := sharedComputer(t)
	if err := ctx.CancelAddLearner(); err != nil || ctx.Learner() != first || ctx.Learners.Find(first.ID) == nil {
		t.Fatalf("cancel with nothing added: %v, playing %v", err, ctx.Learner())
	}
	if _, err := ctx.AddLearner("Cara"); err != nil {
		t.Fatal(err)
	}
	n := len(ctx.Learners.List)
	if err := ctx.CancelAddLearner(); err != nil || ctx.Learner() != first || len(ctx.Learners.List) != n-1 {
		t.Fatalf("cancel: %v, playing %v, %d learners", err, ctx.Learner(), len(ctx.Learners.List))
	}
	if err := ctx.CancelAddLearner(); err != nil || ctx.Learner() != first || len(ctx.Learners.List) != n-1 {
		t.Fatalf("second cancel changed things: %v, playing %v", err, ctx.Learner())
	}
}

// A learner signed in to an account is not a safe one to land on after a
// removal: their tokens would send the next child's answers to it (#38).
func TestRemoveSkipsLinked(t *testing.T) {
	ctx, first, locked := sharedComputer(t)
	locked.Lock = nil
	first.LearnerID = "lrn_1" // linked, no lock
	other, err := ctx.AddLearner("Cara")
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.RemoveLearner(other.ID); err != nil {
		t.Fatal(err)
	}
	cur := ctx.Learner()
	if cur == nil || cur.LearnerID != "" || cur.ID == first.ID {
		t.Fatalf("playing %+v", cur)
	}
}

// Removing a learner who isn't playing leaves the one playing alone.
func TestRemoveOtherKeepsPlayer(t *testing.T) {
	ctx, first, locked := sharedComputer(t)
	if err := ctx.RemoveLearner(locked.ID); err != nil {
		t.Fatal(err)
	}
	if ctx.Learner() != first || ctx.Learners.Find(locked.ID) != nil {
		t.Fatalf("playing %v", ctx.Learner())
	}
}
