package scene

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/input"
	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/pkg/compete"
	"github.com/halpworld/halpwords/pkg/maps"
	"github.com/halpworld/halpwords/pkg/words"
)

// savePath is where the test save slot lives on disk.
func savePath(t *testing.T) string {
	t.Helper()
	for _, dir := range []string{os.Getenv("XDG_CONFIG_HOME"), os.Getenv("HOME")} {
		var found string
		filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err == nil && d.Name() == saveName {
				found = p
			}
			return nil
		})
		if found != "" {
			return found
		}
	}
	t.Fatal("save file not found")
	return ""
}

type saveState int

const (
	noSave saveState = iota
	goodSave
	corruptSave
	unreadableSave
)

func (s saveState) String() string {
	return [...]string{"no save", "a save", "a corrupt save", "an unreadable save"}[s]
}

func arrange(t *testing.T, ctx *game.Context, s saveState) {
	t.Helper()
	switch s {
	case goodSave:
		writeTestSave(t, ctx)
	case corruptSave:
		if err := save.Write(saveName, []byte("{not json")); err != nil {
			t.Fatal(err)
		}
	case unreadableSave:
		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("cannot make an unreadable file here")
		}
		if err := save.Write(saveName, []byte("{}")); err != nil {
			t.Fatal(err)
		}
		p := savePath(t)
		if err := os.Chmod(p, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(p, 0o600) })
	}
}

// Every way of reaching the class picker ends in the same question, so each
// is tried with every kind of save slot.
func TestEveryRouteIntoClassPickAsksOnlyWithASave(t *testing.T) {
	fr, _ := words.Lookup("fr")
	routes := map[string]func(ctx *game.Context, next func() game.Scene) *ClassPick{
		"quest": func(ctx *game.Context, next func() game.Scene) *ClassPick {
			s, msg := questStart(ctx, &maps.Quest{Language: "fr"})
			if s == nil {
				t.Fatalf("quest did not start: %s", msg)
			}
			// A quest that names no lists asks which to play (#89).
			lp := s.(*ListPick)
			press(t, ebiten.KeyEnter)
			if err := lp.Update(ctx); err != nil {
				t.Fatal(err)
			}
			return next().(*ClassPick)
		},
		"adventure": func(ctx *game.Context, next func() game.Scene) *ClassPick {
			a := NewAdventure(ctx, runSetup{mode: compete.Adventure}).(*Adventure)
			press(t, ebiten.KeyEnter)
			if err := a.Update(ctx); err != nil {
				t.Fatal(err)
			}
			lp := next().(*ListPick)
			if err := lp.Update(ctx); err != nil { // Enter: the same lists as last time
				t.Fatal(err)
			}
			return next().(*ClassPick)
		},
		"daily": func(ctx *game.Context, next func() game.Scene) *ClassPick {
			a := NewAdventure(ctx, runSetup{mode: compete.Daily}).(*Adventure)
			press(t, ebiten.KeyEnter)
			if err := a.Update(ctx); err != nil {
				t.Fatal(err)
			}
			return next().(*ClassPick)
		},
		"direct hardcore": func(ctx *game.Context, _ func() game.Scene) *ClassPick {
			return NewClassPick(fr, runSetup{mode: compete.Hardcore}).(*ClassPick)
		},
	}
	for name, route := range routes {
		for _, st := range []saveState{noSave, goodSave, corruptSave} {
			t.Run(name+"/"+st.String(), func(t *testing.T) {
				useTempDir(t)
				ctx := testContext(t)
				ctx.Input = &input.State{}
				next := ctx.TestScenes(nil)
				arrange(t, ctx, st)
				p := route(ctx, next)
				next() // drain
				press(t, ebiten.KeyEnter)
				if err := p.Update(ctx); err != nil {
					t.Fatal(err)
				}
				started := next()
				if st == noSave {
					if p.confirming() || started == nil {
						t.Fatal("must start at once without a save")
					}
					return
				}
				if !p.confirming() || started != nil {
					t.Fatalf("with %s the run must not start without asking (confirming %v, started %v)",
						st, p.confirming(), started)
				}
			})
		}
	}
}

func askingPick(t *testing.T, ctx *game.Context) (*ClassPick, func() game.Scene) {
	t.Helper()
	ctx.Input = &input.State{}
	next := ctx.TestScenes(nil)
	fr, _ := words.Lookup("fr")
	p := NewClassPick(fr, runSetup{mode: compete.Adventure}).(*ClassPick)
	press(t, ebiten.KeyEnter)
	if err := p.Update(ctx); err != nil {
		t.Fatal(err)
	}
	if !p.confirming() {
		t.Fatal("not asking")
	}
	return p, next
}

func TestClassPickConfirmKeys(t *testing.T) {
	for _, tc := range []struct {
		key   ebiten.Key
		start bool
	}{
		{ebiten.KeyY, true},
		{ebiten.KeyN, false},
		{ebiten.KeyEscape, false},
		{ebiten.KeyEnter, false}, // the default is No
		{ebiten.KeySpace, false},
	} {
		useTempDir(t)
		ctx := testContext(t)
		writeTestSave(t, ctx)
		before, _ := save.Read(saveName)
		p, next := askingPick(t, ctx)
		press(t, tc.key)
		if err := p.Update(ctx); err != nil {
			t.Fatal(err)
		}
		_, isCrawl := next().(*Crawl)
		if isCrawl != tc.start {
			t.Errorf("key %v: started %v, want %v", tc.key, isCrawl, tc.start)
		}
		if !tc.start && p.confirming() {
			t.Errorf("key %v: still asking", tc.key)
		}
		if after, _ := save.Read(saveName); string(after) != string(before) {
			t.Errorf("key %v: the save changed", tc.key)
		}
	}
}

// Esc at the question goes back to the cards, not out of the picker; after
// No the question can be asked again.
func TestClassPickEscStaysInThePicker(t *testing.T) {
	useTempDir(t)
	ctx := testContext(t)
	writeTestSave(t, ctx)
	p, next := askingPick(t, ctx)
	press(t, ebiten.KeyEscape)
	p.Update(ctx)
	if next() != nil || p.confirming() {
		t.Fatal("Esc must only close the question")
	}
	press(t, ebiten.KeyEnter)
	p.Update(ctx)
	if !p.confirming() || p.yes {
		t.Fatal("asking again must default to No")
	}
}

// A save whose class can't be read still has the player asked.
func TestSavedHeroOfACorruptSave(t *testing.T) {
	useTempDir(t)
	save.Write(saveName, []byte("garbage"))
	if h, _, ok := savedHero(); !ok || h != "" {
		t.Fatalf("got %q %v", h, ok)
	}
	if replaceText() == "" {
		t.Fatal("no text")
	}
}
