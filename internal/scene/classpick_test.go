package scene

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/pkg/compete"
	"github.com/halpworld/halpwords/pkg/words"
)

// press makes key the only one pressed on the next Update.
func press(t *testing.T, key ebiten.Key) {
	t.Helper()
	input.FakeKeys(t, func(k ebiten.Key) int {
		if k == key {
			return 1
		}
		return 0
	})
}

func writeTestSave(t *testing.T, ctx *game.Context) {
	t.Helper()
	r, l := testRun(t, ctx)
	data, err := encodeSave(r, l, l.Start, dungeon.North, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := save.Write(saveName, data); err != nil {
		t.Fatal(err)
	}
}

// Starting a run with a save on disk must ask first (#48), and the
// answer must default to keeping the save.
func TestClassPickConfirmsBeforeReplacingASave(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	ctx.Input = &input.State{}
	next := ctx.TestScenes(nil)
	lang, _ := words.Lookup("fr")
	writeTestSave(t, ctx)
	before, _ := save.Read(saveName)

	step := func(key ebiten.Key, p *ClassPick) {
		t.Helper()
		press(t, key)
		if err := p.Update(ctx); err != nil {
			t.Fatal(err)
		}
	}
	p := NewClassPick(lang, runSetup{mode: compete.Hardcore}).(*ClassPick)
	step(ebiten.KeyEnter, p)
	if !p.confirming() || next() != nil {
		t.Fatal("Enter with a save on disk started a run without asking")
	}
	step(ebiten.KeyEnter, p) // the default answer
	if p.confirming() || next() != nil {
		t.Fatal("the default answer must be No, and keep the save")
	}
	step(ebiten.KeyEnter, p)
	step(ebiten.KeyEscape, p)
	if p.confirming() || next() != nil {
		t.Fatal("Esc must answer No")
	}
	step(ebiten.KeyEnter, p)
	step(ebiten.KeyArrowDown, p) // to Yes
	step(ebiten.KeyEnter, p)
	if _, ok := next().(*Crawl); !ok {
		t.Fatal("Yes did not start the run")
	}
	if after, _ := save.Read(saveName); string(after) != string(before) {
		t.Fatal("the save changed before the new run saved anything")
	}
}

func TestClassPickAsksNothingWithoutASave(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	ctx.Input = &input.State{}
	next := ctx.TestScenes(nil)
	lang, _ := words.Lookup("fr")
	p := NewClassPick(lang, runSetup{mode: compete.Adventure}).(*ClassPick)
	press(t, ebiten.KeyEnter)
	if err := p.Update(ctx); err != nil {
		t.Fatal(err)
	}
	if p.confirming() {
		t.Fatal("asked to replace a save that is not there")
	}
	if _, ok := next().(*Crawl); !ok {
		t.Fatal("did not start the run")
	}
}

func TestSavedHeroNamesTheHeroAndFloor(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	if _, _, ok := savedHero(); ok {
		t.Fatal("found a save in an empty folder")
	}
	writeTestSave(t, ctx)
	hero, floor, ok := savedHero()
	if !ok || hero != "Rogue" || floor != 3 {
		t.Fatalf("got %q floor %d ok %v, want Rogue floor 3", hero, floor, ok)
	}
}

// The prompt shows what the save is, mode included, and says when a
// Hardcore or Daily run can't be got back.
func TestReplaceTextShowsTheModeOfTheSave(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	c := hardcoreCrawl(t, ctx, 4)
	if !c.writeSave(ctx, true) {
		t.Fatal("could not suspend")
	}
	want := "This replaces your saved adventure: French · Knight · Floor 4 · Hardcore"
	if got := replaceText(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if !savedScored() {
		t.Fatal("a Hardcore save was not seen as scored")
	}
	useTempDir(t)
	writeTestSave(t, ctx)
	if savedScored() {
		t.Fatal("an Adventure save was seen as scored")
	}
}
