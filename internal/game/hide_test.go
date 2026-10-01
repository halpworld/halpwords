package game

import (
	"reflect"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

type hideScene struct {
	log *[]string
}

func (hideScene) Update(*Context) error        { return nil }
func (hideScene) Draw(*ebiten.Image, *Context) {}
func (s hideScene) OnClose(*Context)           { *s.log = append(*s.log, "hider close") }
func (s hideScene) OnHide(*Context)            { *s.log = append(*s.log, "hider hide") }

// A hidden page tells a Hider OnHide, and any other Closer OnClose; a
// closed page tells both OnClose. A panic doesn't stop the rest on hide.
func TestSaveOnHideChoosesTheLighterSave(t *testing.T) {
	var log []string
	ctx := &Context{}
	next := ctx.TestScenes(&closeScene{name: "plain", log: &log})
	ctx.Push(hideScene{log: &log})
	next()
	ctx.SaveOnHide()
	if want := []string{"hider hide", "plain"}; !reflect.DeepEqual(log, want) {
		t.Fatalf("hide: got %v, want %v", log, want)
	}
	log = nil
	ctx.SaveOnClose()
	if want := []string{"hider close", "plain"}; !reflect.DeepEqual(log, want) {
		t.Fatalf("close: got %v, want %v", log, want)
	}
	log = nil
	ctx.Push(panicScene{})
	next()
	ctx.SaveOnHide()
	if want := []string{"hider hide", "plain"}; !reflect.DeepEqual(log, want) {
		t.Fatalf("hide with a panic: got %v, want %v", log, want)
	}
}
