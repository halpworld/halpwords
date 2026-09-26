//go:build !js

package profile

import (
	"errors"
	"io/fs"
	"testing"
	"time"

	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/pkg/words"
)

// TestMigrateFirstLearner: a game from before W2.5 kept one player's
// files in the user's folder; they move into the first learner's folder,
// and the computer's own files stay.
func TestMigrateFirstLearner(t *testing.T) {
	useTempDir(t)
	t.Cleanup(func() { save.Use(save.Root) })
	old := map[string]string{
		"settings.json":       `{}`,
		"progress.json":       `{"Memory":{}}`,
		"halloffame.json":     `{"Name":"Ada"}`,
		"adventure.json":      `{}`,
		"link.json":           `{"Refresh":"hwr_1"}`,
		"words/animals.txt":   "title: Animals\n",
		"assigned/lst_1.txt":  "title: Colours\n",
		"quests/cave.hwquest": "{}",
		"ai.json":             `{"key":"secret"}`,
		"ai/bank-fr.json":     `{}`,
		"crash.txt":           "boom",
		"reports/queue.json":  `[]`,
	}
	for n, d := range old {
		if err := save.Root.Write(n, []byte(d)); err != nil {
			t.Fatal(err)
		}
	}
	ls, err := OpenLearners(save.Root)
	if err != nil {
		t.Fatal(err)
	}
	if len(ls.List) != 1 || ls.Current != ls.List[0].ID || ls.List[0].Name != "Ada" || !ls.Migrated {
		t.Fatalf("learners: %+v", ls)
	}
	f := ls.List[0].Folder()
	for n, d := range old {
		mine := !shared(n)
		got, err := f.Read(n)
		if mine && (err != nil || string(got) != d) {
			t.Errorf("%s in the learner's folder: %q, %v", n, got, err)
		}
		if !mine && err == nil {
			t.Errorf("%s, the computer's, moved into the learner's folder", n)
		}
		_, err = save.Root.Read(n)
		if mine != errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s in the user's folder: %v", n, err)
		}
	}
	// Opening again changes nothing.
	again, err := OpenLearners(save.Root)
	if err != nil || len(again.List) != 1 || again.Current != ls.Current {
		t.Fatalf("again: %+v, %v", again, err)
	}
}

func TestNewComputerHasOneLearner(t *testing.T) {
	useTempDir(t)
	ls, err := OpenLearners(save.Root)
	if err != nil {
		t.Fatal(err)
	}
	if len(ls.List) != 1 || ls.List[0].Name != "Player 1" || ls.CurrentLearner() == nil {
		t.Fatalf("learners: %+v", ls.List)
	}
}

// TestLearnersKeptApart: switching learners switches every file.
func TestLearnersKeptApart(t *testing.T) {
	useTempDir(t)
	t.Cleanup(func() { save.Use(save.Root) })
	ls, err := OpenLearners(save.Root)
	if err != nil {
		t.Fatal(err)
	}
	a := ls.CurrentLearner()
	if err := ls.Use(a.ID); err != nil {
		t.Fatal(err)
	}
	pa, _ := Load()
	pa.Name = "Aoife"
	pa.MemoryFor("fr").Record(words.Entry{Prompt: "dog", Answers: []string{"le chien"}}, words.Answer{Tier: words.Perfect})
	pa.SaveFame()
	pa.SaveMemory()
	save.Write("words/mine.txt", []byte("title: Mine\n"))

	b, err := ls.Add("Bríd")
	if err != nil {
		t.Fatal(err)
	}
	if err := ls.Use(b.ID); err != nil {
		t.Fatal(err)
	}
	pb, errs := Load()
	if len(errs) > 0 || pb.Name != "" || len(pb.Memory) != 0 {
		t.Fatalf("the new learner sees %q, %v, %v", pb.Name, pb.Memory, errs)
	}
	if names, _ := save.List("words"); len(names) != 0 {
		t.Errorf("the new learner sees lists %v", names)
	}
	// A profile loaded for a learner writes to their folder, whoever
	// plays later.
	pa.Name = "Aoife B"
	pa.SaveFame()
	if pb2, _ := Load(); pb2.Name != "" {
		t.Errorf("Aoife's save went to Bríd's folder: %q", pb2.Name)
	}
	if err := ls.Use(a.ID); err != nil {
		t.Fatal(err)
	}
	pa2, _ := Load()
	if pa2.Name != "Aoife B" || len(pa2.MemoryFor("fr").Cards) != 1 {
		t.Errorf("Aoife's profile: %q, %v", pa2.Name, pa2.MemoryFor("fr").Cards)
	}

	// The list survives a restart, and remembers who played last.
	again, err := OpenLearners(save.Root)
	if err != nil || len(again.List) != 2 || again.Current != a.ID {
		t.Fatalf("again: %+v, %v", again, err)
	}

	// Removing a learner removes their files.
	if err := again.Remove(b.ID); err != nil {
		t.Fatal(err)
	}
	if names, _ := b.Folder().All(); len(names) != 0 {
		t.Errorf("left after removing: %v", names)
	}
	if len(again.List) != 1 {
		t.Errorf("list: %+v", again.List)
	}
}

func TestDamagedListIsRebuilt(t *testing.T) {
	useTempDir(t)
	ls, _ := OpenLearners(save.Root)
	b, _ := ls.Add("")
	b.Folder().Write("settings.json", []byte("{}"))
	ls.CurrentLearner().Folder().Write("settings.json", []byte("{}"))
	ls.Save()
	save.Root.Write(learnersFile, []byte("{nope"))
	again, err := OpenLearners(save.Root)
	if err != nil || len(again.List) != 2 {
		t.Fatalf("rebuilt: %+v, %v", again, err)
	}
}

func TestLock(t *testing.T) {
	useTempDir(t)
	ls, _ := OpenLearners(save.Root)
	now := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	ls.now = func() time.Time { return now }
	l := ls.CurrentLearner()
	if ok, err := ls.Unlock(l.ID, "anything"); !ok || err != nil {
		t.Fatalf("no lock: %v, %v", ok, err)
	}
	lock, err := NewLock(LockPictures, PicturesSecret([]int{4, 0, 7}))
	if err != nil {
		t.Fatal(err)
	}
	l.Lock = lock
	if lock.Hash == "" || lock.Salt == "" || lock.Opens(PicturesSecret([]int{4, 7, 0})) {
		t.Fatalf("lock: %+v", lock)
	}
	for i := 1; i < MaxTries; i++ {
		if ok, err := ls.Unlock(l.ID, "1,2,3"); ok || err != nil {
			t.Fatalf("try %d: %v, %v", i, ok, err)
		}
	}
	if ok, err := ls.Unlock(l.ID, "1,2,3"); ok || !errors.Is(err, ErrLocked) {
		t.Fatalf("try %d: %v, %v", MaxTries, ok, err)
	}
	if ok, err := ls.Unlock(l.ID, "4,0,7"); ok || !errors.Is(err, ErrLocked) {
		t.Fatalf("while locked: %v, %v", ok, err)
	}
	now = now.Add(LockFor)
	if ok, err := ls.Unlock(l.ID, "4,0,7"); !ok || err != nil {
		t.Fatalf("after the lockout: %v, %v", ok, err)
	}
	// The lock survives a restart.
	again, _ := OpenLearners(save.Root)
	if again.CurrentLearner().Lock == nil || !again.CurrentLearner().Lock.Opens("4,0,7") {
		t.Error("the lock was lost")
	}
}
