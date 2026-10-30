package profile

import (
	"github.com/halpworld/halpwords/internal/save"
	"os"
	"testing"
)

// The learner being unlocked has an unwritable folder: their own identity
// copy is best effort, so the right code still opens (learners.json saved).
func TestUnlockOpensWhenOwnIdentityUnwritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes anywhere")
	}
	ls, ada := openIdentityFixture(t)
	dir, err := ada.Folder().Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })
	if ok, err := ls.Unlock(ada.ID, "WRONG"); ok || err != nil {
		t.Errorf("wrong code: %v, %v", ok, err)
	}
	if ok, err := ls.Unlock(ada.ID, "ABCD"); !ok || err != nil {
		t.Errorf("right code with an unwritable own folder: %v, %v", ok, err)
	}
}

// Another learner's folder is read-only through a whole lock cycle: wrong
// tries, a lockout and a correct code all work for the learner whose
// folder is fine, and the list reopens with their tries.
func TestOtherLearnersReadOnlyFolderThroughLockCycle(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes anywhere")
	}
	ls, ada := openIdentityFixture(t)
	bo, _ := ls.Add("Bo")
	bo.LearnerID = "lrn_bo" // would need an identity.json
	dir, err := bo.Folder().Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })
	for i := 0; i < MaxTries-1; i++ {
		if ok, err := ls.Unlock(ada.ID, "NOPE"); ok || err != nil {
			t.Fatalf("wrong try %d: %v, %v", i, ok, err)
		}
	}
	if ok, err := ls.Unlock(ada.ID, "NOPE"); ok || err != ErrLocked {
		t.Fatalf("last wrong try: %v, %v", ok, err)
	}
	got, err := OpenLearners(save.Root)
	if err != nil {
		t.Fatal(err)
	}
	if a := got.Find(ada.ID); a == nil || a.LockedUntil.IsZero() {
		t.Errorf("lockout not on the list: %+v", a)
	}
	if b := got.Find(bo.ID); b == nil || b.LearnerID != "lrn_bo" {
		t.Errorf("Bo lost from learners.json: %+v", b)
	}
}

// An old game's list: locks and names in learners.json, no identity
// files. The first Open writes them, so damage right after the upgrade
// loses nothing.
func TestUpgradeThenImmediateDamage(t *testing.T) {
	ls, ada := openIdentityFixture(t)
	bo, _ := ls.Add("Bo")
	if err := ls.Save(); err != nil {
		t.Fatal(err)
	}
	for _, l := range ls.List {
		save.Root.Remove(idPath(l.ID))
	}
	save.Root.Write(idPathOf(bo.ID, "progress.json"), []byte("{}"))
	if _, err := OpenLearners(save.Root); err != nil { // the upgrade start
		t.Fatal(err)
	}
	got := reopenAfterDamage(t)
	if a := got.Find(ada.ID); a == nil || a.Lock == nil || a.Name != "Ada" {
		t.Errorf("Ada after upgrade+damage: %+v", a)
	}
	if b := got.Find(bo.ID); b == nil || b.Name != "Bo" {
		t.Errorf("Bo after upgrade+damage: %+v", b)
	}
}

func TestGeneratedNameEdges(t *testing.T) {
	for n, want := range map[string]bool{
		"Player 07": true, "Player 1": true, "Player 1 ": false, " Player 1": false,
		"Player  1": false, "player 1": false, "Player 1a": false, "Player ٣": false,
		"Player -1": false, "Player": false, "": false, "Player 1\n": false,
	} {
		if got := GeneratedName(n); got != want {
			t.Errorf("GeneratedName(%q) = %v, want %v", n, got, want)
		}
	}
}

// A name a child typed with edge forms of the generated shape: "Player 1 "
// is trimmed by Add, so it becomes generated-looking and gets no file.
func TestTypedNamesLookingGenerated(t *testing.T) {
	ls, _ := openIdentityFixture(t)
	p, _ := ls.Add("Player 1 ") // Add trims
	ls.Save()
	if p.Name != "Player 1" {
		t.Fatalf("name %q", p.Name)
	}
	q, _ := ls.Add("Player 07")
	ls.Save()
	// Both are indistinguishable from the game's own names: no identity
	// file, as documented; rebuilt as a "Player N".
	for _, l := range []*Learner{p, q} {
		if _, err := save.Root.Read(idPath(l.ID)); err == nil {
			t.Errorf("%q got an identity file", l.Name)
		}
	}
}
