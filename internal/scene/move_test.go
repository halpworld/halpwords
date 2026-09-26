package scene

import (
	"io/fs"
	"maps"
	"slices"
	"testing"

	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/move"
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
