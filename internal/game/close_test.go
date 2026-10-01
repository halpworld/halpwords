package game

import (
	"reflect"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

type closeScene struct {
	name string
	log  *[]string
}

func (s *closeScene) Update(*Context) error        { return nil }
func (s *closeScene) Draw(*ebiten.Image, *Context) {}
func (s *closeScene) OnClose(*Context)             { *s.log = append(*s.log, s.name) }

type plainScene struct{}

func (plainScene) Update(*Context) error        { return nil }
func (plainScene) Draw(*ebiten.Image, *Context) {}

// Closing the window or the page tells every scene that keeps something,
// the top one first, including those under a menu pushed on top (#49).
func TestSaveOnCloseReachesTheSceneStack(t *testing.T) {
	var log []string
	ctx := &Context{}
	next := ctx.TestScenes(&closeScene{name: "run", log: &log})
	ctx.Push(plainScene{})
	next()
	ctx.Push(&closeScene{name: "overlay", log: &log})
	next()
	ctx.SaveOnClose()
	if want := []string{"overlay", "run"}; !reflect.DeepEqual(log, want) {
		t.Fatalf("got %v, want %v", log, want)
	}
}

type panicScene struct{ plainScene }

func (panicScene) OnClose(*Context) { panic("boom") }

// One scene that panics must not stop the others saving.
func TestSaveOnCloseSurvivesAPanickingScene(t *testing.T) {
	dir := t.TempDir() // the crash report goes here, not to the user's folder
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("AppData", dir)
	var log []string
	ctx := &Context{}
	next := ctx.TestScenes(&closeScene{name: "run", log: &log})
	ctx.Push(panicScene{})
	next()
	ctx.SaveOnClose()
	if want := []string{"run"}; !reflect.DeepEqual(log, want) {
		t.Fatalf("got %v, want %v", log, want)
	}
}
