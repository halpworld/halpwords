package game

import (
	"errors"
	"strings"
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
	// Aoife has played: the next child doesn't land in her Hall of Fame.
	if cur := ctx.Learner(); cur == first || cur == nil || ctx.Profile.Name != "" {
		t.Errorf("after removing, playing %v named %q", ctx.Learner(), ctx.Profile.Name)
	}
	if names, _ := second.Folder().All(); len(names) > 0 {
		t.Errorf("removed learner's files are left: %v", names)
	}
	if len(ctx.Learners.List) != 2 {
		t.Errorf("%d learners, want 2 (Aoife and a new guest)", len(ctx.Learners.List))
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

// putTokens gives a learner's folder a sign-in, as a game that linked has.
func putTokens(t *testing.T, l *profile.Learner, way string) {
	t.Helper()
	data := `{"Server":"http://127.0.0.1:1","Refresh":"hwr_x","Way":"` + way + `","NextSeq":1}`
	if err := l.Folder().WritePrivate("link.json", []byte(data)); err != nil {
		t.Fatal(err)
	}
}

// A learner holding tokens is signed in whatever LearnerID says (it is
// filled in only once the server has been heard): removal must not land on
// them (#38).
func TestRemoveSkipsTokensWithoutLearnerID(t *testing.T) {
	ctx, first, _ := sharedComputer(t)
	putTokens(t, first, "pairing") // lock-free, LearnerID empty
	other, err := ctx.AddLearner("Cara")
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.RemoveLearner(other.ID); err != nil {
		t.Fatal(err)
	}
	if cur := ctx.Learner(); cur == nil || cur.ID == first.ID || hasTokens(cur) {
		t.Fatalf("playing %+v: a learner with tokens", cur)
	}
}

// The next child never lands in someone's hero, name or Hall of Fame: a
// learner with progress isn't a guest, a pristine one is reused, and
// guests don't pile up (#38).
func TestGuestIsPristineAndReused(t *testing.T) {
	ctx, first, _ := sharedComputer(t)
	first.Folder().Write("halloffame.json", []byte(`{"Name":"Ada"}`))
	if ctx.Pristine(first) {
		t.Fatal("a learner with a Hall of Fame is pristine")
	}
	typed, _ := ctx.Learners.Add("Ada") // a name somebody typed
	if ctx.Pristine(typed) {
		t.Error("a learner with a typed name is pristine")
	}
	a, _ := ctx.AddLearner("Cara")
	ctx.RemoveLearner(a.ID)
	g1 := ctx.Learner()
	if g1 == first || g1 == typed || !ctx.Pristine(g1) {
		t.Fatalf("landed on %+v", g1)
	}
	n := len(ctx.Learners.List)
	b, _ := ctx.AddLearner("Dan")
	ctx.RemoveLearner(b.ID)
	if ctx.Learner() != g1 || len(ctx.Learners.List) != n {
		t.Errorf("playing %v of %d learners: a second guest was made", ctx.Learner(), len(ctx.Learners.List))
	}
}

// Start up on the learner who played last unless they have to sign in:
// then a guest plays and the Switch learner screen comes first. A
// grown-up's pairing-code learner resumes (#38).
func TestStartupRestore(t *testing.T) {
	for _, c := range []struct {
		name   string
		setup  func(t *testing.T, l *profile.Learner)
		resume bool
	}{
		{"lock", func(t *testing.T, l *profile.Learner) { l.Lock, _ = profile.NewLock(profile.LockCard, "A") }, false},
		{"card tokens, no lock", func(t *testing.T, l *profile.Learner) { putTokens(t, l, "card") }, false},
		{"school account", func(t *testing.T, l *profile.Learner) { putTokens(t, l, "sso") }, false},
		{"pairing code", func(t *testing.T, l *profile.Learner) { putTokens(t, l, "pairing") }, true},
		{"plain", func(t *testing.T, l *profile.Learner) {}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx, first, _ := sharedComputer(t)
			c.setup(t, first)
			ctx.Learners.Current = first.ID
			if err := ctx.Learners.Save(); err != nil {
				t.Fatal(err)
			}
			ctx.Link.Close()
			again := &Context{Sound: &Sound{Muted: true}}
			again.openLearners()
			cur := again.Learner()
			if cur == nil {
				t.Fatal("nobody playing")
			}
			if c.resume != (cur.ID == first.ID) || c.resume == again.TakeNeedWho() {
				t.Errorf("resume %v: playing %q (first %q)", c.resume, cur.ID, first.ID)
			}
			if !c.resume && again.NeedsSignIn(cur) {
				t.Errorf("started on a learner who has to sign in")
			}
		})
	}
}

// A learner signed in with a school account has no lock to open them
// with: nothing switches to them, and they are never a guest (#38).
func TestSchoolAccountLearnerNotEntered(t *testing.T) {
	ctx, first, locked := sharedComputer(t)
	locked.Lock = nil
	locked.NeedsSignIn = true
	if err := ctx.SwitchLearner(locked.ID); !errors.Is(err, ErrLearnerLocked) {
		t.Fatalf("SwitchLearner = %v", err)
	}
	if ok, _ := ctx.UnlockLearner(locked.ID, ""); ok {
		t.Fatal("unlocked a learner with nothing to open them")
	}
	other, _ := ctx.AddLearner("Cara")
	ctx.RemoveLearner(other.ID)
	if cur := ctx.Learner(); cur == locked || cur == nil || !ctx.Pristine(cur) {
		t.Fatalf("playing %+v, first %v", cur, first.ID)
	}
}

// After a sign-out at school that keeps the progress, the game leaves
// the learner even without a lock, and they can't be chosen again (#38).
func TestSignOutKeepAtSchoolLeaves(t *testing.T) {
	ctx, _, _ := sharedComputer(t)
	pupil, _ := ctx.Learners.Add("Sam")
	// The school lets the progress stay.
	data := `{"Server":"http://127.0.0.1:1","Refresh":"hwr_x","Way":"sso","NextSeq":1,"Me":{"keep_on_sign_out":true}}`
	if err := pupil.Folder().WritePrivate("link.json", []byte(data)); err != nil {
		t.Fatal(err)
	}
	if err := ctx.switchLearner(pupil.ID); err != nil {
		t.Fatal(err)
	}
	if !ctx.Link.Linked() || !ctx.Link.KeepOnSignOut() {
		t.Fatal("not linked, or the progress doesn't stay")
	}
	done := ctx.SignOut()
	for !done() {
	}
	if cur := ctx.Learner(); cur == nil || cur == pupil || !ctx.Pristine(cur) {
		t.Fatalf("playing %+v after a school sign-out", cur)
	}
	if !ctx.NeedsSignIn(pupil) || ctx.SwitchLearner(pupil.ID) == nil {
		t.Error("the signed-out pupil can be chosen without signing in")
	}
}

// If the game can't switch to a guest, nobody plays: not the learner who
// signed out (#38).
func TestFailClosed(t *testing.T) {
	ctx, _, locked := sharedComputer(t)
	if _, err := ctx.UnlockLearner(locked.ID, "ABCD"); err != nil {
		t.Fatal(err)
	}
	ctx.leaveErr = errors.New("disk full")
	done := ctx.SignOut()
	for !done() {
	}
	if ctx.Learner() != nil || ctx.Learners.Current != "" {
		t.Fatalf("playing %+v", ctx.Learner())
	}
	if save.Current() == locked.Folder() {
		t.Error("still saving in the signed-out learner's folder")
	}
	// And removal fails closed too.
	ctx.leaveErr = nil
	if err := ctx.SwitchLearner(ctx.Learners.List[0].ID); err != nil {
		t.Fatal(err)
	}
	ctx.leaveErr = errors.New("disk full")
	if err := ctx.RemoveLearner(ctx.Learners.Current); err == nil || ctx.Learner() != nil {
		t.Fatalf("remove: %v, playing %v", err, ctx.Learner())
	}
}

// A damaged list is made again from the folders, without the locks:
// learners holding tokens have to sign in again (#38).
func TestRebuildNeedsSignIn(t *testing.T) {
	ctx, first, _ := sharedComputer(t)
	putTokens(t, first, "pairing")
	ctx.Link.Close()
	if err := save.Root.Write("learners.json", []byte("{damaged")); err != nil {
		t.Fatal(err)
	}
	again := &Context{Sound: &Sound{Muted: true}}
	again.openLearners()
	if again.notice == "" {
		t.Error("no notice that the data was repaired")
	}
	got := again.Learners.Find(first.ID)
	if got == nil || !got.NeedsSignIn || again.SwitchLearner(got.ID) == nil && again.Learner() == got {
		t.Errorf("rebuilt learner %+v can be chosen without signing in", got)
	}
	if cur := again.Learner(); cur == nil || cur.ID == first.ID {
		t.Errorf("started on %+v", cur)
	}
}

// A learner whose link.json is damaged counts as holding tokens: they
// have to sign in, and are never a guest (#38).
func TestDamagedLinkJSONNeedsSignIn(t *testing.T) {
	ctx, first, _ := sharedComputer(t)
	first.Folder().WritePrivate("link.json", []byte(`{"Refresh":"hwr_x","Way":"pairing","NextSeq":"oops"}`))
	if !ctx.NeedsSignIn(first) || ctx.Pristine(first) {
		t.Error("a damaged link.json hides a sign-in")
	}
}

// A sign-in with a school account waiting for the website keeps its
// learner at start-up if it started less than 10 minutes ago; later, or
// with tokens, a guest plays (#38).
func TestStartupResumesRecentSSO(t *testing.T) {
	for _, c := range []struct {
		name    string
		ago     time.Duration
		tokens  bool
		resumes bool
	}{
		{"just started", time.Minute, false, true},
		{"just under the limit", 9*time.Minute + 50*time.Second, false, true},
		{"too long ago", 11 * time.Minute, false, false},
		{"has tokens", time.Minute, true, false},
		{"no start time", 0, false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx, first, _ := sharedComputer(t)
			sso := `{"device_code":"d","user_code":"u","expires":"` + time.Now().Add(time.Hour).Format(time.RFC3339) + `"`
			if c.ago > 0 {
				sso += `,"started":"` + time.Now().Add(-c.ago).Format(time.RFC3339) + `"`
			}
			sso += `}`
			refresh := ""
			if c.tokens {
				refresh = `"Refresh":"hwr_x","Way":"sso",`
			}
			data := `{"Server":"http://127.0.0.1:1",` + refresh + `"NextSeq":1,"SSO":` + sso + `}`
			first.Folder().WritePrivate("link.json", []byte(data))
			ctx.Learners.Current = first.ID
			ctx.Learners.Save()
			ctx.Link.Close()
			again := &Context{Sound: &Sound{Muted: true}}
			again.openLearners()
			cur := again.Learner()
			if cur == nil || (cur.ID == first.ID) != c.resumes {
				t.Errorf("resumes %v: playing %+v", c.resumes, cur)
			}
		})
	}
}

// The same learner signing in again opens their own folder; somebody
// else gets a new learner and the old folder is left alone (#38).
func TestSignInAgainAdoptsSameFolder(t *testing.T) {
	for _, same := range []bool{true, false} {
		ctx, _, _ := sharedComputer(t)
		old, _ := ctx.Learners.Add("Sam")
		old.NeedsSignIn, old.Owner = true, "lrn_1"
		old.Folder().Write("halloffame.json", []byte(`{"Name":"Sam"}`))
		ctx.Learners.Save()
		added, err := ctx.AddLearner("")
		if err != nil {
			t.Fatal(err)
		}
		// The sign-in happens, and the server says who it was.
		who := "lrn_1"
		if !same {
			who = "lrn_2"
		}
		ctx.Link.Close()
		data := `{"Server":"http://127.0.0.1:1","Refresh":"hwr_new","Way":"sso","NextSeq":5,"Me":{"learner":{"id":"` + who + `","display_name":"Sam"}}}`
		if err := added.Folder().WritePrivate("link.json", []byte(data)); err != nil {
			t.Fatal(err)
		}
		if err := ctx.switchLearner(added.ID); err != nil {
			t.Fatal(err)
		}
		ctx.SignInFor(old.ID)
		ctx.notePlayer()
		if same {
			if ctx.Learner() != old || ctx.Learners.Find(added.ID) != nil || !ctx.Link.Linked() {
				t.Fatalf("same learner: playing %+v, new learner kept: %v", ctx.Learner(), ctx.Learners.Find(added.ID) != nil)
			}
			if got, _ := old.Folder().Read("halloffame.json"); !strings.Contains(string(got), "Sam") {
				t.Error("their progress is gone")
			}
			if old.Owner != "lrn_1" {
				t.Errorf("owner %q", old.Owner)
			}
			continue
		}
		if ctx.Learner() != added || ctx.Learners.Find(old.ID) == nil {
			t.Fatalf("somebody else: playing %+v", ctx.Learner())
		}
		if _, err := old.Folder().Read("link.json"); err == nil {
			t.Error("the old folder got somebody else's sign-in")
		}
		if !old.NeedsSignIn {
			t.Error("the old learner can be chosen without signing in")
		}
	}
}

// failClosed clears what a game with nobody playing left in the folder of
// nobody's, and leaves nobody playing; cancelling an add from there
// removes the new learner again (#38).
func TestFailClosedClearsSinkAndCancelAdd(t *testing.T) {
	ctx, _, _ := sharedComputer(t)
	sinkFolder.Write("halloffame.json", []byte(`{"Name":"Old"}`))
	ctx.failClosed("")
	if names, _ := sinkFolder.All(); len(names) > 0 {
		t.Errorf("left in the folder of nobody's: %v", names)
	}
	n := len(ctx.Learners.List)
	added, err := ctx.AddLearner("")
	if err != nil {
		t.Fatal(err)
	}
	if !ctx.AddPending() {
		t.Fatal("no add to cancel")
	}
	if err := ctx.CancelAddLearner(); err != nil {
		t.Fatal(err)
	}
	if ctx.Learner() != nil || len(ctx.Learners.List) != n || ctx.Learners.Find(added.ID) != nil {
		t.Errorf("playing %v with %d learners, want nobody and %d", ctx.Learner(), len(ctx.Learners.List), n)
	}
}

// If the learners can't be read and the folder of the game from before
// still holds a login, the game doesn't play as them (#38).
func TestNoLearnersDoesNotOpenLegacyLogin(t *testing.T) {
	useTempDir(t)
	save.Root.WritePrivate("link.json", []byte(`{"Refresh":"hwr_x","Way":"card","NextSeq":1}`))
	ctx := &Context{Sound: &Sound{Muted: true}}
	ctx.noLearners(errors.New("boom"))
	if save.Current() == save.Root || ctx.Learners != nil {
		t.Errorf("playing in %q", save.Current())
	}
	save.Use(save.Root)
}

func linkJSON(who string) []byte {
	return []byte(`{"Server":"http://127.0.0.1:1","Refresh":"hwr_new","Way":"sso","NextSeq":5,"Me":{"learner":{"id":"` + who + `","display_name":"Sam"}}}`)
}

// A sign-in begun on a new learner is dropped on a switch to somebody
// else: when Me arrives for them, they are not folded into the old
// folder, and the old folder stays as it was (#38).
func TestSwitchAwayDropsSignInAgain(t *testing.T) {
	ctx, _, _ := sharedComputer(t)
	target, _ := ctx.Learners.Add("Sam")
	target.NeedsSignIn, target.Owner = true, "lrn_1"
	ctx.Learners.Save()
	if _, err := ctx.AddLearner(""); err != nil {
		t.Fatal(err)
	}
	ctx.SignInFor(target.ID)
	// Somebody else, the same server learner, with progress.
	y, _ := ctx.Learners.Add("Yan")
	y.Folder().Write("halloffame.json", []byte(`{"Name":"Yan"}`))
	y.Folder().WritePrivate("link.json", linkJSON("lrn_1"))
	ctx.Learners.Save()
	if err := ctx.switchLearner(y.ID); err != nil {
		t.Fatal(err)
	}
	ctx.notePlayer()
	if ctx.Learner() != y || ctx.Learners.Find(y.ID) == nil {
		t.Fatalf("playing %+v", ctx.Learner())
	}
	if got, _ := y.Folder().Read("halloffame.json"); !strings.Contains(string(got), "Yan") {
		t.Error("their progress is gone")
	}
	if _, err := y.Folder().Read("link.json"); err != nil {
		t.Error("their login moved")
	}
	if !target.NeedsSignIn {
		t.Error("the old folder changed")
	}
	if _, err := target.Folder().Read("link.json"); err == nil {
		t.Error("the old folder got a login")
	}
}

// A sign-in again without a lock (a pairing code, or SSO) keeps the lock
// the folder had (#38).
func TestSignInAgainKeepsLock(t *testing.T) {
	ctx, _, _ := sharedComputer(t)
	old, _ := ctx.Learners.Add("Sam")
	var err error
	if old.Lock, err = profile.NewLock(profile.LockCard, "ABCD"); err != nil {
		t.Fatal(err)
	}
	old.NeedsSignIn, old.Owner = true, "lrn_1"
	ctx.Learners.Save()
	added, err := ctx.AddLearner("")
	if err != nil {
		t.Fatal(err)
	}
	ctx.SignInFor(old.ID)
	ctx.Link.Close()
	added.Folder().WritePrivate("link.json", linkJSON("lrn_1"))
	if err := ctx.switchLearner(added.ID); err != nil {
		t.Fatal(err)
	}
	ctx.SignInFor(old.ID)
	ctx.notePlayer()
	if ctx.Learner() != old {
		t.Fatalf("playing %+v", ctx.Learner())
	}
	if old.Lock == nil || !ctx.NeedsSignIn(old) {
		t.Errorf("lock lost: %+v", old)
	}
}

// Parked events of a learner who doesn't come back are dropped once
// they expire, at start-up (halpworld/halpwords#39).
func TestStartupSweepsParked(t *testing.T) {
	useTempDir(t)
	ctx := &Context{Sound: &Sound{Muted: true}}
	ctx.openLearners()
	if ctx.Learners == nil {
		t.Fatal("no learners")
	}
	brian, err := ctx.AddLearner("Brian")
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-31 * 24 * time.Hour).UTC().Format(time.RFC3339)
	if err := brian.Folder().WritePrivate("link-parked.json", []byte(`[{"Learner":"lrn_1","At":"`+old+`","Queue":{"Answers":[{"Seq":1}]}}]`)); err != nil {
		t.Fatal(err)
	}
	again := &Context{Sound: &Sound{Muted: true}}
	again.openLearners()
	if _, err := brian.Folder().Read("link-parked.json"); err == nil {
		t.Error("expired parked events kept in a learner's folder")
	}
}
