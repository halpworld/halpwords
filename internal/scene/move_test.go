package scene

import (
	"errors"
	"io/fs"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/move"
	"github.com/halpworld/halpwords/internal/save"
)

type memStore map[string][]byte

func (m memStore) Read(name string) ([]byte, error) {
	if d, ok := m[name]; ok {
		return d, nil
	}
	return nil, fs.ErrNotExist
}
func (m memStore) Write(name string, data []byte) error { m[name] = data; return nil }
func (m memStore) Remove(name string) error             { delete(m, name); return nil }
func (m memStore) All() ([]string, error)               { return slices.Sorted(maps.Keys(m)), nil }

func noLeave(t *testing.T) func(string) error {
	return func(u string) error { t.Fatalf("left for %s", u); return nil }
}

// Progress brought to a browser that already has some asks first, and
// keeping what is here leaves it alone.
func TestStartAsksBeforeReplacing(t *testing.T) {
	payload, _ := move.Encode(map[string][]byte{"adventure.json": []byte("old hero")})
	frag := move.Split(payload, move.MaxPart)[0].Fragment()
	st := memStore{"adventure.json": []byte("new hero")}
	ctx := &game.Context{}
	ask, ok := startWith(ctx, st, frag, noLeave(t)).(*moveAsk)
	if !ok {
		t.Fatal("didn't ask")
	}
	if err := ask.in.Discard(st); err != nil {
		t.Fatal(err)
	}
	if string(st["adventure.json"]) != "new hero" {
		t.Fatal("replaced without asking")
	}
	// Following the old link again doesn't ask again.
	if _, asked := startWith(ctx, st, frag, noLeave(t)).(*moveAsk); asked {
		t.Fatal("asked twice")
	}
}

// A big save comes in parts: the game fetches the next from the old page.
func TestStartFetchesTheNextPart(t *testing.T) {
	payload, _ := move.Encode(map[string][]byte{"settings.json": []byte("{}")})
	parts := move.Split(payload, 10)
	var went string
	s := startWith(&game.Context{}, memStore{}, parts[0].Fragment(), func(u string) error { went = u; return nil })
	if m, ok := s.(*moving); !ok || m.part != 2 {
		t.Fatalf("got %T", s)
	}
	if went != move.OldURL+"#send=2" {
		t.Fatalf("went to %q", went)
	}
}

// Progress brought to an empty browser goes through the busy Importing
// screen, so the unlinking runs off the game loop.
func TestStartImportsOnABusyScreen(t *testing.T) {
	useTempDir(t)
	payload, _ := move.Encode(map[string][]byte{"adventure.json": []byte("hero")})
	frag := move.Split(payload, move.MaxPart)[0].Fragment()
	st := memStore{}
	s := startWith(&game.Context{}, st, frag, noLeave(t))
	imp, ok := s.(*importing)
	if !ok {
		t.Fatalf("got %T", s)
	}
	if len(st["adventure.json"]) != 0 {
		t.Fatal("applied before the old progress was let go")
	}
	select {
	case <-imp.done:
	case <-time.After(5 * time.Second):
		t.Fatal("never finished preparing")
	}
}

// When letting go of the old link failed, nothing is applied (the old
// tokens might be reopened on the new progress) and the title comes next.
func TestImportingRefusesToApplyOnError(t *testing.T) {
	useTempDir(t)
	t.Cleanup(func() { save.Use(save.Root) }) // opening the learners picks a learner's folder
	ctx := testContext(t)
	ctx.TestOpen()
	next := ctx.TestScenes(nil)
	st := memStore{"adventure.json": []byte("here")}
	in := &move.Incoming{ID: "0123456789abcdef", Files: map[string][]byte{"adventure.json": []byte("brought")}}
	done := make(chan error, 1)
	done <- errors.New("moving: boom")
	imp := &importing{st: st, in: in, done: done}
	if err := imp.Update(ctx); err != nil {
		t.Fatal(err)
	}
	if string(st["adventure.json"]) != "here" || len(st) != 1 {
		t.Fatalf("applied anyway: %v", st)
	}
	if _, ok := next().(*Title); !ok {
		t.Fatalf("next scene %T, want the title", next())
	}
}
