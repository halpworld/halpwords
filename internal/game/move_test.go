package game

import (
	"strings"
	"testing"

	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/move"
	"github.com/halpworld/halpwords/internal/save"
)

const liveTokens = `{"DeviceID":"d1","Access":"a","Refresh":"r"}`

// moveInto brings files in the way the game does: prepare, apply, reload.
func moveInto(t *testing.T, ctx *Context, files map[string][]byte) {
	t.Helper()
	ctx.PrepareMove(files)
	in := &move.Incoming{ID: "0123456789abcdef", Files: files}
	if err := in.Apply(move.Saves); err != nil {
		t.Fatal(err)
	}
	if err := ctx.ReloadSaves(); err != nil {
		t.Fatal(err)
	}
}

// computerWithLinkedLearner is a computer whose current learner is linked,
// with a second learner who is too.
func computerWithLinkedLearner(t *testing.T) (ctx *Context, a, b save.Folder) {
	useTempDir(t)
	ctx = &Context{Sound: &Sound{Muted: true}}
	ctx.openLearners()
	ctx.openLink()
	ctx.loadProfile()
	t.Cleanup(func() { ctx.Link.Close() })
	a = ctx.Learner().Folder()
	second, err := ctx.AddLearner("Brian")
	if err != nil {
		t.Fatal(err)
	}
	b = second.Folder()
	if err := ctx.SwitchLearner(ctx.Learners.List[0].ID); err != nil {
		t.Fatal(err)
	}
	for _, f := range []save.Folder{a, b} {
		f.WritePrivate("link.json", []byte(liveTokens))
		f.Write("link-queue.json", []byte(`{}`))
		f.WritePrivate("link-parked.json", []byte(`[]`))
		f.Write("progress.json", []byte(`{}`))
	}
	return ctx, a, b
}

// Progress that moves in while the game runs puts the game where a
// start-up on those saves would: the imported learner plays, in their own
// folder, the list isn't overwritten, and the learners who were here have
// no tokens left on disk.
func TestMoveReplacesTheLearnersAndTheirLinks(t *testing.T) {
	ctx, a, b := computerWithLinkedLearner(t)
	old := ctx.Link
	list := `{"List":[{"ID":"imp1","Name":"Aoife"}],"Current":"imp1","Migrated":true}`
	moveInto(t, ctx, map[string][]byte{
		"learners.json":               []byte(list),
		"profiles/imp1/progress.json": []byte(`{}`),
	})
	if l := ctx.Learner(); l == nil || l.ID != "imp1" || save.Current() != l.Folder() {
		t.Fatalf("playing %v in %q, want imp1", ctx.Learner(), save.Current())
	}
	if ctx.Link == old {
		t.Error("the link of the learner who was playing is still the game's")
	}
	// Saving the list now keeps the imported one.
	if err := ctx.Learners.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := save.Root.Read("learners.json")
	if !strings.Contains(string(data), "imp1") || len(ctx.Learners.List) != 1 {
		t.Errorf("learners.json = %s (%d learners)", data, len(ctx.Learners.List))
	}
	for _, f := range []save.Folder{a, b} {
		if p := link.PeekFolder(f); p.Tokens {
			t.Errorf("%s still holds tokens", f)
		}
		if names, _ := f.All(); len(names) > 0 {
			t.Errorf("%s still holds %v", f, names)
		}
	}
}

// With no list of learners in what comes, the folders that were here
// aren't made learners again, and their tokens go.
func TestMoveWithoutLearnersListLeavesNoLinks(t *testing.T) {
	ctx, a, b := computerWithLinkedLearner(t)
	moveInto(t, ctx, map[string][]byte{"settings.json": []byte(`{}`)})
	for _, f := range []save.Folder{a, b} {
		if p := link.PeekFolder(f); p.Tokens {
			t.Errorf("%s still holds tokens", f)
		}
	}
	for _, l := range ctx.Learners.List {
		if l.Folder() == a || l.Folder() == b {
			t.Errorf("%s came back as a learner", l.Folder())
		}
	}
	if ctx.Learner() == nil || save.Current() != ctx.Learner().Folder() {
		t.Errorf("playing %v in %q", ctx.Learner(), save.Current())
	}
}
