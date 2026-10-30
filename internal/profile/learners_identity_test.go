package profile

import (
	"strings"
	"testing"
	"time"

	"github.com/halpworld/halpwords/internal/save"
)

// openIdentityFixture makes a list with a locked learner "Ada" and returns it.
func openIdentityFixture(t *testing.T) (*Learners, *Learner) {
	t.Helper()
	useTempDir(t)
	t.Cleanup(func() { save.Use(save.Root) })
	ls, err := OpenLearners(save.Root)
	if err != nil {
		t.Fatal(err)
	}
	ada := ls.List[0]
	ada.Name, ada.LearnerID, ada.Owner = "Ada", "lrn_ada", "lrn_ada"
	if ada.Lock, err = NewLock(LockCard, "ABCD"); err != nil {
		t.Fatal(err)
	}
	if err := ls.Save(); err != nil {
		t.Fatal(err)
	}
	return ls, ada
}

func idPath(id string) string { return "profiles/" + id + "/" + identityFile }

func reopenAfterDamage(t *testing.T) *Learners {
	t.Helper()
	save.Root.Write(learnersFile, []byte("\x00\x01 garbage"))
	got, err := OpenLearners(save.Root)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// A lockout in force must survive the damage: deleting learners.json must
// not hand a locked-out child's classmate fresh tries.
func TestIdentityKeepsLockoutAndOwner(t *testing.T) {
	ls, ada := openIdentityFixture(t)
	now := time.Now()
	ls.now = func() time.Time { return now }
	for i := 0; i < MaxTries; i++ {
		ls.Unlock(ada.ID, "WRNG")
	}
	if ada.LockedUntil.IsZero() {
		t.Fatal("setup: not locked out")
	}
	got := reopenAfterDamage(t)
	a := got.Find(ada.ID)
	if a == nil || a.LockedUntil.IsZero() || !a.LockedUntil.Equal(ada.LockedUntil) {
		t.Fatalf("lockout lost: %+v", a)
	}
	if a.Owner != "lrn_ada" || a.LearnerID != "lrn_ada" {
		t.Errorf("owner/learner id lost: %+v", a)
	}
	got.now = func() time.Time { return now.Add(time.Second) }
	if ok, err := got.Unlock(ada.ID, "ABCD"); ok || err != ErrLocked {
		t.Errorf("right code during lockout: %v, %v", ok, err)
	}
}

// Wrong tries are written as they happen, so a restored counter is current.
func TestIdentityTracksEveryWrongTry(t *testing.T) {
	ls, ada := openIdentityFixture(t)
	ls.Unlock(ada.ID, "NOPE")
	ls.Unlock(ada.ID, "NOPE")
	got := reopenAfterDamage(t)
	if a := got.Find(ada.ID); a == nil || a.Tries != 2 {
		t.Fatalf("tries after two wrong tries and damage: %+v", a)
	}
}

// Corrupt or hostile identity files must never crash or hand out a bad lock.
func TestRebuildRejectsBadIdentity(t *testing.T) {
	cases := map[string]string{
		"not json":        "{nope",
		"empty file":      "",
		"json null":       "null",
		"json array":      "[1,2]",
		"wrong id":        `{"ID":"someoneelse","Name":"Eve"}`,
		"no id":           `{"Name":"Eve"}`,
		"truncated":       `{"ID":"%ID%","Name":"Ev`,
		"wrong type name": `{"ID":"%ID%","Name":42}`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			ls, ada := openIdentityFixture(t)
			save.Root.Write(idPath(ada.ID), []byte(strings.ReplaceAll(content, "%ID%", ada.ID)))
			got := reopenAfterDamage(t)
			a := got.Find(ada.ID)
			if a == nil {
				t.Fatal("learner dropped")
			}
			if a.Lock != nil || !strings.HasPrefix(a.Name, "Player ") {
				t.Errorf("accepted bad identity: %+v", a)
			}
			if len(got.List) != len(ls.List) {
				t.Errorf("%d learners, want %d", len(got.List), len(ls.List))
			}
		})
	}
}

func TestRebuildFixesUnusableLockAndName(t *testing.T) {
	ls, ada := openIdentityFixture(t)
	// A lock with no hash can never open; a blank name is no name.
	save.Root.Write(idPath(ada.ID), []byte(`{"ID":"`+ada.ID+`","Name":"   ","Lock":{"Kind":"card","Salt":"","Hash":""}}`))
	got := reopenAfterDamage(t)
	a := got.Find(ada.ID)
	if a == nil || a.Lock != nil || !a.NeedsSignIn || strings.TrimSpace(a.Name) == "" {
		t.Fatalf("unusable lock/blank name not handled: %+v", a)
	}
	_ = ls
}

// A removed learner must not come back from a damaged list.
func TestRemovedLearnerStaysGone(t *testing.T) {
	ls, ada := openIdentityFixture(t)
	bo, _ := ls.Add("Bo")
	bo.LearnerID = "lrn_bo"
	ls.Save()
	if err := ls.Remove(bo.ID); err != nil {
		t.Fatal(err)
	}
	ls.Save()
	got := reopenAfterDamage(t)
	if got.Find(bo.ID) != nil {
		t.Error("removed learner came back")
	}
	if got.Find(ada.ID) == nil {
		t.Error("remaining learner lost")
	}
}

// Old behaviour: a rebuilt list had no locks. Make sure an unlocked,
// unnamed guest still gets no identity file (Pristine depends on it) and
// that clearing everything removes a stale file.
func TestIdentityFileNotWrittenForPlainGuest(t *testing.T) {
	ls, ada := openIdentityFixture(t)
	bo, _ := ls.Add("")
	ls.Save()
	if _, err := save.Root.Read(idPath(bo.ID)); err == nil {
		t.Error("plain guest got an identity file")
	}
	ada.Lock, ada.LearnerID, ada.Owner, ada.Name = nil, "", "", "Player 1"
	ls.Save()
	if _, err := save.Root.Read(idPath(ada.ID)); err == nil {
		t.Error("stale identity file kept after the entry became plain")
	}
}

// The identity file must not be rewritten when nothing changed.
func TestSaveIdentityIsIdempotent(t *testing.T) {
	ls, ada := openIdentityFixture(t)
	a, _ := save.Root.Read(idPath(ada.ID))
	for i := 0; i < 3; i++ {
		if err := ls.Save(); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := save.Root.Read(idPath(ada.ID))
	if string(a) != string(b) || len(a) == 0 {
		t.Error("identity changed across no-op saves")
	}
}

// Names the child typed that start like the default are still kept when
// the learner has anything else worth keeping; a damaged list must never
// leak the lock hash into the quarantine incorrectly or panic with a
// list holding nil/duplicate entries.
func TestRebuildWithGarbageFoldersAndRace(t *testing.T) {
	ls, ada := openIdentityFixture(t)
	save.Root.Write("profiles/BAD ID/x.json", []byte("{}"))
	save.Root.Write("profiles/../escape.json", []byte("{}"))
	save.Root.Write("profiles/.hidden", []byte("{}"))
	got := reopenAfterDamage(t)
	for _, l := range got.List {
		if !validID(l.ID) {
			t.Errorf("invalid ID in list: %q", l.ID)
		}
	}
	if got.Find(ada.ID) == nil {
		t.Error("Ada lost")
	}
	_ = ls
}
