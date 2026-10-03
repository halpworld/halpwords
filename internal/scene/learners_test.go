package scene

import (
	"encoding/json"
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	"image"
	"strings"
	"testing"

	"github.com/halpworld/halpwords/assets"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/gfx"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/internal/unifont"
	"github.com/halpworld/halpwords/pkg/proc"
)

func TestShowLongCode(t *testing.T) {
	for _, c := range []struct {
		code string
		n    int
		want string
	}{
		{"", link.CardCodeLen, "____-____-____"},
		{"ABCDEF", link.CardCodeLen, "ABCD-EF__-____"},
		{"C1A55C0D", link.ClassCodeLen, "C1A5-5C0D"},
	} {
		if got := showLongCode([]rune(c.code), c.n); got != c.want {
			t.Errorf("showLongCode(%q, %d) = %q, want %q", c.code, c.n, got, c.want)
		}
	}
}

// Every picture the server may send has an image to show.
func TestPictureImages(t *testing.T) {
	for i := range proc.PictureCount {
		if pictureImage(i) == nil {
			t.Errorf("no image for picture %d", i)
		}
	}
	if pictureImage(proc.PictureCount) != nil || pictureImage(-1) != nil {
		t.Error("an image for a picture that doesn't exist")
	}
}

// The picture keys on the keypad are the numbers on the labels, not the
// keypad's layout (#47).
func TestPictureKeysMatchLabels(t *testing.T) {
	for i, keys := range pictureKeys {
		want := fmt.Sprint(i + 1)
		for _, k := range keys {
			if got := k.String(); !strings.HasSuffix(got, want) {
				t.Errorf("picture %s has key %v", want, got)
			}
		}
	}
}

// A letter in the class's name list only jumps: W and S are also Up and
// Down in menus, and must not move the selection again (#46).
func TestNameListLetterOnlyJumps(t *testing.T) {
	names := []string{"Aoife", "Brian", "Sam", "Will", "Zoe"}
	if got := jumpName(names, 0, []rune("s")); got != 2 {
		t.Errorf("s: %d, want 2 (Sam)", got)
	}
	if got := jumpName(names, 0, []rune("W")); got != 3 {
		t.Errorf("W: %d, want 3 (Will)", got)
	}
	if got := jumpName(names, 3, []rune("q")); got != 3 {
		t.Errorf("no name with q moved to %d", got)
	}
}

// updateName jumps to the name a typed letter starts, and with no arrow
// key down stays there: the screen shows the same name the child asked
// for (#46). (Arrow keys can't be faked: ebiten reads them itself.)
func TestUpdateNameLetterSelects(t *testing.T) {
	var class link.Class
	if err := json.Unmarshal([]byte(`{"learners":[{"id":"a","display_name":"Aoife"},{"id":"b","display_name":"Brian"},{"id":"s","display_name":"Sam"},{"id":"w","display_name":"Will"}]}`), &class); err != nil {
		t.Fatal(err)
	}
	ctx := testContext(t)
	ctx.Input = &input.State{}
	s := &SignIn{step: siName, class: &class}
	for _, c := range []struct {
		chars string
		want  int
	}{{"s", 2}, {"w", 3}, {"S", 2}, {"b", 3 - 2}, {"x", 1}} {
		ctx.Input.Chars = []rune(c.chars)
		s.updateName(ctx)
		if s.sel != c.want {
			t.Fatalf("after %q: selected %d, want %d", c.chars, s.sel, c.want)
		}
	}
}

// Typing S or W jumps to the name and stays there: the keys are Down and
// Up in menus, but in the name list they only type (#46). The arrow keys
// still move.
func TestUpdateNameSWOnlyJump(t *testing.T) {
	var class link.Class
	if err := json.Unmarshal([]byte(`{"learners":[{"id":"a","display_name":"Aoife"},{"id":"b","display_name":"Brian"},{"id":"s","display_name":"Sam"},{"id":"w","display_name":"Will"}]}`), &class); err != nil {
		t.Fatal(err)
	}
	var held ebiten.Key = -1
	input.FakeKeys(t, func(k ebiten.Key) int {
		if k == held {
			return 1
		}
		return 0
	})
	ctx := testContext(t)
	ctx.Input = &input.State{}
	for _, c := range []struct {
		key   ebiten.Key
		chars string
		from  int
		want  int
	}{
		{ebiten.KeyS, "s", 0, 2},
		{ebiten.KeyW, "w", 0, 3},
		{ebiten.KeyArrowDown, "", 1, 2},
		{ebiten.KeyArrowUp, "", 1, 0},
	} {
		held = c.key
		ctx.Input.Chars = []rune(c.chars)
		s := &SignIn{step: siName, class: &class, sel: c.from}
		s.updateName(ctx)
		if s.sel != c.want {
			t.Errorf("key %v typing %q from %d: selected %d, want %d", c.key, c.chars, c.from, s.sel, c.want)
		}
	}
}

func learnersCtx(t *testing.T) *game.Context {
	t.Helper()
	useTempDir(t)
	ctx := &game.Context{Sound: &game.Sound{Muted: true}}
	ctx.TestOpen()
	if ctx.Learners == nil {
		t.Fatal("no learners")
	}
	t.Cleanup(func() {
		ctx.Link.Close()
		save.Use(save.Root)
	})
	return ctx
}

// The notes beside the rows are worked out when the list changes, and
// Draw reads them as they were (#38).
func TestLearnersNotesCached(t *testing.T) {
	ctx := learnersCtx(t)
	other, err := ctx.AddLearner("Sam")
	if err != nil {
		t.Fatal(err)
	}
	other.NeedsSignIn = true
	ctx.Learners.Save()
	if err := ctx.SwitchLearner(ctx.Learners.List[0].ID); err != nil {
		t.Fatal(err)
	}
	s := NewLearners(ctx).(*Learners)
	s.rows(ctx)
	if s.notes[other.ID] != "sign in to play" || s.notes[ctx.Learner().ID] != "playing now (guest)" {
		t.Errorf("notes %v", s.notes)
	}
	// Nothing in the key moved, so a change under the screen isn't looked at.
	other.NeedsSignIn = false
	s.rows(ctx)
	if s.notes[other.ID] != "sign in to play" {
		t.Errorf("worked out again without a change: %v", s.notes)
	}
	s.changed()
	s.rows(ctx)
	if s.notes[other.ID] == "sign in to play" {
		t.Errorf("not worked out after a change: %v", s.notes)
	}
}

// A learner who needs to sign in and has no lock can be removed from the
// screen, after a confirmation, but not the learner playing (#38).
func TestLearnersRemoveStranded(t *testing.T) {
	ctx := learnersCtx(t)
	playing := ctx.Learner()
	other, err := ctx.AddLearner("Sam")
	if err != nil {
		t.Fatal(err)
	}
	other.NeedsSignIn = true
	ctx.Learners.Save()
	if err := ctx.SwitchLearner(playing.ID); err != nil {
		t.Fatal(err)
	}
	s := NewLearners(ctx).(*Learners)
	var rm *lrRow
	for _, r := range s.rows(ctx) {
		if r.action == lrRemoveOther && r.learner == other {
			rm = &r
		}
		if r.action == lrRemoveOther && r.learner == playing {
			t.Error("a Remove row for the learner playing")
		}
	}
	if rm == nil {
		t.Fatal("no Remove row for the stranded learner")
	}
	s.confirm, s.victim = lrRemoveOther, other
	s.doConfirmed(ctx)
	if ctx.Learners.Find(other.ID) != nil || ctx.Learners.Find(playing.ID) == nil {
		t.Errorf("learners after: %v", ctx.Learners.List)
	}
}

// With nobody chosen, the Account screen goes to Switch learner and the
// keys on it do nothing (#38).
func TestAccountWithNobodyGoesToSwitch(t *testing.T) {
	ctx := learnersCtx(t)
	a := NewAccount(ctx)
	next := ctx.TestScenes(a)
	ctx.Learners.Current = ""
	ctx.Input = &input.State{}
	if err := a.Update(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := next().(*Learners); !ok {
		t.Errorf("Account stayed")
	}
}

// Each sign-in picture sits inside its own cell, beside its number and
// inside the panel, and is drawn at the size it was made, so nine
// pictures never cover each other, the numbers or the panel edge
// (F-U4-01: they were drawn twice too big).
func TestPictureGridFits(t *testing.T) {
	panel := picturePanel()
	if !panel.In(image.Rect(0, 0, game.ScreenW, game.ScreenH)) {
		t.Fatalf("panel %v is off the screen", panel)
	}
	for i := range link.Grid {
		r := pictureRect(i)
		c := pictureCellAt(i)
		cell := image.Rectangle{Min: c, Max: c.Add(image.Pt(pictureCell, pictureCell))}
		if !r.In(cell) || !cell.In(panel) {
			t.Errorf("picture %d at %v is outside its cell %v or the panel %v", i+1, r, cell, panel)
		}
		if r.Overlaps(pictureLabel(i)) {
			t.Errorf("picture %d at %v covers its number at %v", i+1, r, pictureLabel(i))
		}
		for j := range i {
			if r.Overlaps(pictureRect(j)) {
				t.Errorf("pictures %d and %d overlap", j+1, i+1)
			}
		}
	}
	if img := pictureImage(0); img == nil || img.Bounds().Size() != pictureRect(0).Size() {
		t.Fatalf("a picture is made at %v but drawn at %v: it would be stretched", img.Bounds().Size(), pictureRect(0).Size())
	}
}

// The picture grid's hint says a number taps (it does, at once) and fits
// on one line (F-U4-02: it said 1-9 choose, Enter tap).
func TestPicturesHint(t *testing.T) {
	face, err := unifont.ParseBytes(assets.UnifontHex)
	if err != nil {
		t.Fatal(err)
	}
	f := gfx.NewFont(face)
	if !strings.HasPrefix(picturesHint, "1-9 tap") {
		t.Errorf("hint %q doesn't say a number taps", picturesHint)
	}
	if w := f.Width(picturesHint, 1); 8+w > game.ScreenW-8 {
		t.Errorf("hint is %d pixels wide: it doesn't fit", w)
	}
}
