package dungeon

import (
	"slices"
	"strings"
	"testing"

	"github.com/halpworld/halpwords/pkg/maps"
	"github.com/halpworld/halpwords/pkg/puzzle"
)

// The map format's lists of monsters and traits are the game's.
func TestMapNamesMatchTheGame(t *testing.T) {
	var kinds, bosses, traits []string
	for _, k := range Kinds {
		kinds = append(kinds, k.Name)
	}
	for _, k := range BossKinds {
		bosses = append(bosses, k.Name)
	}
	for _, tr := range Traits {
		traits = append(traits, strings.ToLower(tr.String()))
	}
	if !slices.Equal(kinds, maps.MonsterKinds) || !slices.Equal(bosses, maps.BossKinds) || !slices.Equal(traits, maps.Traits) {
		t.Errorf("maps lists %v %v %v, the game has %v %v %v", maps.MonsterKinds, maps.BossKinds, maps.Traits, kinds, bosses, traits)
	}
	for _, name := range traits {
		if TraitNamed(name) == 0 || TraitNamed(strings.ToUpper(name)) == 0 {
			t.Errorf("no trait named %q", name)
		}
	}
	if TraitNamed("") != 0 || TraitNamed("sleepy") != 0 {
		t.Error("a trait for an unknown name")
	}
}

// Every generated floor is a map that passes the checks, and building a
// floor from that map gives the same floor back: the generator and the map
// rules agree.
func TestGeneratedFloorsAreMaps(t *testing.T) {
	for seed := uint64(1); seed <= 150; seed++ {
		for _, depth := range []int{1, 2, 3, 6, 9, 14} {
			g := Generate(seed, depth)
			m := ToMap(g, "fr")
			if ps := m.Check(nil); len(ps) > 0 {
				t.Fatalf("seed %d depth %d: %v\n%s", seed, depth, ps, strings.Join(m.Rows, "\n"))
			}
			f, err := FromMap(m, seed)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(f.tiles, g.tiles) || !slices.Equal(f.Torches, g.Torches) {
				t.Fatalf("seed %d depth %d: the tiles differ", seed, depth)
			}
			if f.Start != g.Start || f.StartDir != g.StartDir || f.Exit != g.Exit || f.Depth != depth {
				t.Fatalf("seed %d depth %d: start %v %v exit %v, want %v %v %v", seed, depth, f.Start, f.StartDir, f.Exit, g.Start, g.StartDir, g.Exit)
			}
			if len(f.Chests) != len(g.Chests) || len(f.Features) != len(g.Features) || len(f.Monsters) != len(g.Monsters) {
				t.Fatalf("seed %d depth %d: things differ", seed, depth)
			}
			for p, c := range g.Chests {
				if f.Chests[p] == nil || f.Chests[p].Mimic != c.Mimic {
					t.Fatalf("seed %d depth %d: chest at %v differs", seed, depth, p)
				}
			}
			for p, ft := range g.Features {
				if f.Features[p] == nil || f.Features[p].Kind != ft.Kind {
					t.Fatalf("seed %d depth %d: feature at %v differs", seed, depth, p)
				}
				if ft.Kind == Merchant && len(f.Features[p].Stock) != StockSize {
					t.Fatalf("seed %d depth %d: merchant has no stock", seed, depth)
				}
			}
			for i, m := range g.Monsters {
				o := f.Monsters[i]
				if o.Kind != m.Kind || o.At != m.At || o.MaxHP != m.MaxHP || o.ATK != m.ATK || o.Traits != m.Traits || o.Facing != m.Facing {
					t.Fatalf("seed %d depth %d: monster %d is %+v, want %+v", seed, depth, i, o, m)
				}
			}
			// And the map survives a trip through its file.
			data, err := m.Encode()
			if err != nil {
				t.Fatal(err)
			}
			back, err := maps.ParseMap(data)
			if err != nil || !slices.Equal(back.Rows, m.Rows) || len(back.Monsters) != len(m.Monsters) {
				t.Fatalf("seed %d depth %d: encoded map reads back wrong: %v", seed, depth, err)
			}
		}
	}
}

// vault is a small hand-made map with one of everything.
func vault() *maps.Map {
	return &maps.Map{
		Format: maps.MapFormat, Version: maps.Version, Title: "The Word Vault", Language: "fr",
		Theme: "Ice Halls", Depth: 4, Facing: "S",
		Rows: []string{
			"##########",
			"#@..#S.C.#",
			"#...+....#",
			"#...##=###",
			"#F..#..M.#",
			"###=#....#",
			"#C.....>.#",
			"####T#####",
		},
		Monsters: []maps.Monster{
			{X: 6, Y: 5, Kind: "Grumpy Rat", Trait: "swift"},
			{X: 6, Y: 6, Kind: "Bone Lord", Level: 6},
		},
		Locks: []maps.Lock{
			{X: 6, Y: 3, Puzzle: "anagram", Word: "le chat"},
			{X: 7, Y: 1, Mimic: true},
			{X: 1, Y: 6, Puzzle: "tumbler"},
		},
		Notes: []maps.Note{{X: 4, Y: 7, Text: "The cat knows the way."}},
	}
}

func TestFromMap(t *testing.T) {
	f, err := FromMap(vault(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if f.W != 10 || f.H != 8 || f.Depth != 4 || f.Start != (Point{1, 1}) || f.StartDir != South || f.Exit != (Point{7, 6}) {
		t.Fatalf("floor %dx%d depth %d start %v %v exit %v", f.W, f.H, f.Depth, f.Start, f.StartDir, f.Exit)
	}
	if f.At(Point{4, 2}) != Door || f.At(Point{6, 3}) != Sealed || f.At(Point{3, 5}) != Sealed || f.At(Point{7, 6}) != Stairs {
		t.Error("doors or stairs are missing")
	}
	if !f.Torches[f.Index(Point{4, 7})] || f.At(Point{4, 7}) != Wall {
		t.Error("the torch is missing")
	}
	if c := f.Chests[Point{7, 1}]; c == nil || !c.Mimic {
		t.Error("the Mimic chest is missing")
	}
	if c := f.Chests[Point{1, 6}]; c == nil || c.Mimic || c.Gold == 0 {
		t.Errorf("chest %+v", c)
	}
	if f.Features[Point{5, 1}].Kind != Shrine || f.Features[Point{1, 4}].Kind != Campfire || f.Features[Point{7, 4}].Kind != Merchant {
		t.Error("features are missing")
	}
	if len(f.Features[Point{7, 4}].Stock) != StockSize {
		t.Error("the merchant has nothing to sell")
	}
	if l := f.Locks[Point{6, 3}]; !l.Set || l.Kind != puzzle.Anagram || l.Word != "le chat" {
		t.Errorf("door lock %+v", l)
	}
	if l := f.Locks[Point{1, 6}]; !l.Set || l.Kind != puzzle.Tumbler || l.Word != "" {
		t.Errorf("chest lock %+v", l)
	}
	if _, ok := f.Locks[Point{7, 1}]; ok {
		t.Error("a Mimic with no puzzle set has a lock")
	}
	if f.Notes[Point{4, 7}] != "The cat knows the way." {
		t.Errorf("notes %v", f.Notes)
	}
	rat, boss := f.MonsterAt(Point{6, 5}), f.MonsterAt(Point{6, 6})
	if rat == nil || !rat.Has(Swift) || rat.Extra != Swift || rat.MaxHP != NewMonster(KindNamed("Grumpy Rat"), 4, Point{}, 0).MaxHP {
		t.Errorf("rat %+v", rat)
	}
	if boss == nil || f.Boss() != boss || boss.MaxHP != NewMonster(KindNamed("Bone Lord"), 6, Point{}, 0).MaxHP || boss.Facing != West {
		t.Errorf("boss %+v", boss)
	}
	// The stairs are held until the boss falls, and can be reached.
	if d := f.distances(f.Start, true); d[f.Index(f.Exit)] < 0 {
		t.Error("the stairs cannot be reached")
	}
	// The same map and seed give the same floor.
	g, _ := FromMap(vault(), 7)
	if g.Chests[Point{1, 6}].Gold != f.Chests[Point{1, 6}].Gold || g.Monsters[0].Seed != f.Monsters[0].Seed {
		t.Error("the same seed made a different floor")
	}
	// A played floor saves and restores like a generated one.
	f.Set(Point{4, 2}, OpenDoor)
	s := f.State()
	if err := g.Restore(s); err != nil || g.At(Point{4, 2}) != OpenDoor {
		t.Errorf("restore: %v", err)
	}
	// A floor made from the map writes back as the same map.
	back := ToMap(g, "fr")
	if back.Rows[2][4] != maps.Floor || back.Rows[1] != vault().Rows[1] {
		t.Errorf("rows %q", back.Rows)
	}
	if len(back.Locks) != 3 || len(back.Notes) != 1 || len(back.Monsters) != 2 {
		t.Errorf("back %+v", back)
	}
}

func TestFromMapRefusesBrokenMaps(t *testing.T) {
	m := vault()
	m.Rows[6] = "#C.....#.#" // no stairs
	if _, err := FromMap(m, 1); err == nil || !strings.Contains(err.Error(), "stairs") {
		t.Errorf("err %v", err)
	}
}
