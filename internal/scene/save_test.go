package scene

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/halpworld/halpwords/assets"
	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/words"
)

func testContext(t *testing.T) *game.Context {
	t.Helper()
	lists, err := words.LoadFS(assets.Words, "words")
	if err != nil {
		t.Fatal(err)
	}
	return &game.Context{Lists: lists}
}

// A saved run must load back exactly as it was, down to the next random
// number.
func TestSaveRoundTrip(t *testing.T) {
	ctx := testContext(t)
	lang, _ := words.Lookup("fr")
	r := startRun(ctx, lang, 99)
	r.depth = 3
	r.hero.Gold, r.hero.Potions, r.hero.Streak = 17, 2, 4
	r.saved.Gold = 5
	r.seenTraits = dungeon.Swift | dungeon.Armored
	r.say("A test line.", pal.Lime)
	for n := 0; n < 5; n++ {
		_, id := r.deck.Next()
		r.deck.Mark(id, n%2 == 0)
	}
	l := dungeon.Generate(r.floorSeed(r.depth), r.depth)
	l.Seen[l.Index(l.Start)] = true
	l.Monsters[0].HP = 1

	data, err := encodeSave(r, l, l.Start, dungeon.West)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeSave(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	g := got.run
	switch {
	case g.depth != r.depth || g.seed != r.seed || g.lang != r.lang:
		t.Fatalf("got depth %d seed %d lang %s", g.depth, g.seed, g.lang.Code)
	case g.hero != r.hero || g.saved != r.saved:
		t.Fatalf("got hero %+v checkpoint %+v", g.hero, g.saved)
	case g.seenTraits != r.seenTraits:
		t.Fatalf("got traits %v", g.seenTraits)
	case !reflect.DeepEqual(g.log, r.log):
		t.Fatalf("got log %v", g.log)
	case !reflect.DeepEqual(g.deck.State(), r.deck.State()):
		t.Fatalf("got deck %+v, want %+v", g.deck.State(), r.deck.State())
	case !reflect.DeepEqual(got.level, l):
		t.Fatal("floor differs")
	case got.at != l.Start || got.facing != dungeon.West:
		t.Fatalf("got hero at %v facing %v", got.at, got.facing)
	}
	for n := 0; n < 10; n++ {
		if a, b := r.rng.Uint64(), g.rng.Uint64(); a != b {
			t.Fatalf("random numbers differ after loading: %d, %d", a, b)
		}
	}
}

func TestLoadBadSave(t *testing.T) {
	ctx := testContext(t)
	lang, _ := words.Lookup("la")
	r := startRun(ctx, lang, 1)
	l := dungeon.Generate(r.floorSeed(1), 1)
	good, err := encodeSave(r, l, l.Start, l.StartDir)
	if err != nil {
		t.Fatal(err)
	}
	change := func(f func(s map[string]any)) []byte {
		var s map[string]any
		if err := json.Unmarshal(good, &s); err != nil {
			t.Fatal(err)
		}
		f(s)
		data, _ := json.Marshal(s)
		return data
	}
	for name, data := range map[string][]byte{
		"not json":      []byte("{nope"),
		"old version":   change(func(s map[string]any) { s["Version"] = 0 }),
		"no language":   change(func(s map[string]any) { s["Language"] = "xx" }),
		"no floor":      change(func(s map[string]any) { s["Depth"] = 0 }),
		"wrong floor":   change(func(s map[string]any) { s["Depth"] = 9 }),
		"in a wall":     change(func(s map[string]any) { s["At"] = map[string]int{"X": 0, "Y": 0} }),
		"bad generator": change(func(s map[string]any) { s["RNG"] = "" }),
	} {
		if _, err := decodeSave(ctx, data); err == nil {
			t.Errorf("%s: loaded without an error", name)
		}
	}
	if _, err := decodeSave(ctx, good); err != nil {
		t.Fatalf("good save: %v", err)
	}
}
