package maps_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/halpworld/halpwords/assets"
	"github.com/halpworld/halpwords/pkg/maps"
	"github.com/halpworld/halpwords/pkg/proc"
	"github.com/halpworld/halpwords/pkg/puzzle"
	"github.com/halpworld/halpwords/pkg/words"
)

func TestThemesMatchTheGame(t *testing.T) {
	var names []string
	for _, th := range proc.Themes {
		names = append(names, th.Name)
	}
	if !slices.Equal(names, maps.Themes) {
		t.Errorf("maps.Themes is %v, the game has %v", maps.Themes, names)
	}
}

func TestPuzzleNames(t *testing.T) {
	for _, name := range append(slices.Clone(maps.DoorPuzzles), maps.ChestPuzzles...) {
		if _, ok := maps.PuzzleNames[name]; !ok {
			t.Errorf("%q is not a puzzle", name)
		}
	}
	for _, k := range puzzle.Kinds(puzzle.Door, 99) {
		if !slices.ContainsFunc(maps.DoorPuzzles, func(n string) bool { return maps.PuzzleNames[n] == k }) {
			t.Errorf("doors can have %s puzzles, maps cannot", k)
		}
	}
	for _, k := range puzzle.Kinds(puzzle.Chest, 99) {
		if !slices.ContainsFunc(maps.ChestPuzzles, func(n string) bool { return maps.PuzzleNames[n] == k }) {
			t.Errorf("chests can have %s puzzles, maps cannot", k)
		}
	}
}

// good is a small map that passes every check.
func good() *maps.Map {
	return &maps.Map{
		Format: maps.MapFormat, Version: maps.Version, Title: "Escape Room", Language: "fr",
		Rows: []string{
			"#######",
			"#@..C.#",
			"#.###=#",
			"#.#M..#",
			"#.+..>#",
			"###T###",
		},
		Monsters: []maps.Monster{{X: 4, Y: 4, Kind: "Cave Bat"}},
		Locks:    []maps.Lock{{X: 5, Y: 2, Puzzle: "anagram", Word: "le chat"}, {X: 4, Y: 1, Puzzle: "tumbler"}},
		Notes:    []maps.Note{{X: 3, Y: 5, Text: "Two ways down."}},
	}
}

func starter(t *testing.T) []*words.List {
	t.Helper()
	lists, err := words.LoadFS(assets.Words, "words")
	if err != nil {
		t.Fatal(err)
	}
	return lists
}

func TestGoodMap(t *testing.T) {
	m := good()
	if ps := m.Check(nil); len(ps) > 0 {
		t.Fatalf("problems: %v", ps)
	}
	if ps := m.Check(starter(t)); len(ps) > 0 {
		t.Fatalf("problems with the starter lists: %v", ps)
	}
	if w, h := m.Size(); w != 7 || h != 6 || m.At(1, 1) != maps.Start || m.At(-1, 0) != maps.Wall || m.At(9, 9) != maps.Wall {
		t.Error("Size or At is wrong")
	}
	if m.Level() != 1 || m.ThemeIndex() != -1 {
		t.Error("the defaults are wrong")
	}
	m.Theme = "ice halls"
	if m.ThemeIndex() != 3 {
		t.Error("themes are matched ignoring case")
	}
}

// Each change breaks the good map with a problem mentioning want.
func TestCheckFindsProblems(t *testing.T) {
	cases := []struct {
		name string
		edit func(m *maps.Map)
		want string
	}{
		{"no title", func(m *maps.Map) { m.Title = " " }, "title is empty"},
		{"rude title", func(m *maps.Map) { m.Title = "Stupid room" }, "not allowed"},
		{"language", func(m *maps.Map) { m.Language = "xx" }, "Unknown language"},
		{"theme", func(m *maps.Map) { m.Theme = "Candy Land" }, "Unknown theme"},
		{"depth", func(m *maps.Map) { m.Depth = 99 }, "depth"},
		{"facing", func(m *maps.Map) { m.Facing = "up" }, "facing"},
		{"too small", func(m *maps.Map) { m.Rows = m.Rows[:4] }, "must be from"},
		{"ragged", func(m *maps.Map) { m.Rows[2] = "#.###=" }, "every row"},
		{"unknown cell", func(m *maps.Map) { m.Rows[1] = "#@..C?#" }, "Unknown cell"},
		{"open edge", func(m *maps.Map) { m.Rows[0] = "###.###" }, "edge"},
		{"two starts", func(m *maps.Map) { m.Rows[4] = "#@+..>#" }, "one start"},
		{"no stairs", func(m *maps.Map) { m.Rows[4] = "#.+...#" }, "one stairs"},
		{"door in the open", func(m *maps.Map) { m.Rows[3] = "#.#M+.#" }, "opposite sides"},
		{"stairs shut in", func(m *maps.Map) { m.Rows[4] = "#.+.#>#" }, "stairs cannot be reached"},
		{"chest shut in", func(m *maps.Map) { m.Rows[3] = "#.#MC.#"; m.Rows[4] = "#.+.#>#" }, "cannot be reached"},
		{"sealed start", func(m *maps.Map) { m.Rows[1] = "#@#.C.#"; m.Rows[2] = "#=###=#" }, "way out"},
		{"monster in a wall", func(m *maps.Map) { m.Monsters[0].X = 0 }, "empty floor"},
		{"monster on a chest", func(m *maps.Map) { m.Monsters[0] = maps.Monster{X: 4, Y: 1, Kind: "Cave Bat"} }, "empty floor"},
		{"two monsters", func(m *maps.Map) { m.Monsters = append(m.Monsters, m.Monsters[0]) }, "Two monsters"},
		{"unknown monster", func(m *maps.Map) { m.Monsters[0].Kind = "Dragon" }, "Unknown monster"},
		{"two bosses", func(m *maps.Map) {
			m.Monsters = []maps.Monster{{X: 3, Y: 4, Kind: "Bone Lord"}, {X: 4, Y: 4, Kind: "Slime King"}}
		}, "bosses"},
		{"level", func(m *maps.Map) { m.Monsters[0].Level = -1 }, "level"},
		{"trait", func(m *maps.Map) { m.Monsters[0].Trait = "sleepy" }, "Unknown trait"},
		{"lock on floor", func(m *maps.Map) { m.Locks[0].X = 1 }, "sealed door or a chest"},
		{"two locks", func(m *maps.Map) { m.Locks = append(m.Locks, m.Locks[0]) }, "Two puzzles"},
		{"mimic door", func(m *maps.Map) { m.Locks[0].Mimic = true }, "Only a chest"},
		{"unknown puzzle", func(m *maps.Map) { m.Locks[0].Puzzle = "sudoku" }, "Unknown puzzle"},
		{"chest puzzle on a door", func(m *maps.Map) { m.Locks[0].Puzzle = "tumbler" }, "cannot have this puzzle (tumbler lock)"},
		{"word on pairs", func(m *maps.Map) { m.Locks[0].Puzzle = "pairs" }, "several words"},
		{"word without language", func(m *maps.Map) { m.Language = "" }, "needs a language"},
		{"note in the open", func(m *maps.Map) { m.Notes[0].Y = 4 }, "on a wall"},
		{"note off the map", func(m *maps.Map) { m.Notes[0].Y = 40 }, "on a wall"},
		{"long note", func(m *maps.Map) { m.Notes[0].Text = strings.Repeat("a", 141) }, "the most is"},
		{"link note", func(m *maps.Map) { m.Notes[0].Text = "see www.example.com" }, "not allowed"},
	}
	for _, c := range cases {
		m := good()
		c.edit(m)
		ps := m.Check(nil)
		found := false
		for _, p := range ps {
			found = found || strings.Contains(p.Msg, c.want)
		}
		if !found {
			t.Errorf("%s: problems %v, want one about %q", c.name, ps, c.want)
		}
	}
}

func TestCheckWords(t *testing.T) {
	lists := starter(t)
	cases := []struct {
		lock maps.Lock
		want string // "" for no problem
	}{
		{maps.Lock{X: 5, Y: 2, Word: "cat"}, ""},                             // by its English
		{maps.Lock{X: 5, Y: 2, Word: "  LE   Chat "}, ""},                    // case and spacing
		{maps.Lock{X: 5, Y: 2, Puzzle: "riddle", Word: "le chat"}, ""},       // "cat" has a riddle
		{maps.Lock{X: 5, Y: 2, Word: "le zeppelin"}, "not in the word list"}, // not in the lists
		{maps.Lock{X: 4, Y: 1, Puzzle: "tumbler", Word: "un"}, "cannot be used for this puzzle (tumbler lock)"},
		{maps.Lock{X: 4, Y: 1, Puzzle: "gap-fill"}, "cannot make this puzzle (gap fill)"}, // no ">>" sentences
		{maps.Lock{X: 5, Y: 2, Puzzle: "odd-one-out"}, ""},
	}
	for _, c := range cases {
		m := good()
		m.Locks = []maps.Lock{c.lock}
		ps := m.Check(lists)
		switch {
		case c.want == "" && len(ps) > 0:
			t.Errorf("%+v: problems %v", c.lock, ps)
		case c.want != "" && (len(ps) != 1 || !strings.Contains(ps[0].Msg, c.want)):
			t.Errorf("%+v: problems %v, want one about %q", c.lock, ps, c.want)
		}
	}
	// Lists without groups cannot make odd-one-out puzzles.
	plain := []*words.List{{Language: "fr", Entries: []words.Entry{
		{Prompt: "cat", Answers: []string{"le chat"}}, {Prompt: "dog", Answers: []string{"le chien"}},
	}}}
	m := good()
	m.Locks = []maps.Lock{{X: 5, Y: 2, Puzzle: "odd-one-out"}}
	if ps := m.Check(plain); len(ps) != 1 || !strings.Contains(ps[0].Msg, "cannot make this puzzle (odd one out)") {
		t.Errorf("problems %v", ps)
	}
	// A list with a sentence for a word can make gap-fill puzzles about it.
	withCloze := []*words.List{{Language: "fr", Entries: plain[0].Entries,
		Cloze: []words.ClozeLine{{Sentence: "Je vois ___.", Answer: "le chat"}}}}
	m.Locks = []maps.Lock{{X: 4, Y: 1, Puzzle: "gap-fill", Word: "cat"}}
	if ps := m.Check(withCloze); len(ps) > 0 {
		t.Errorf("problems %v", ps)
	}
	m.Locks[0].Word = "dog"
	if ps := m.Check(withCloze); len(ps) != 1 || !strings.Contains(ps[0].Msg, "“dog” cannot be used") {
		t.Errorf("problems %v", ps)
	}
	// Lists in another language have no words for the map.
	if ps := good().Check([]*words.List{{Language: "la", Entries: plain[0].Entries}}); len(ps) != 1 || !strings.Contains(ps[0].Msg, "no French words") {
		t.Errorf("problems %v", ps)
	}
}

func TestFindWord(t *testing.T) {
	entries := []words.Entry{
		{Prompt: "cat", Answers: []string{"le chat"}},
		{Prompt: "le chat", Answers: []string{"the cat"}}, // an answer wins over a prompt
		{Prompt: "coffee", Answers: []string{"le café", "un café"}},
		{Prompt: "today", Answers: []string{"aujourd'hui"}},
	}
	for word, want := range map[string]int{
		"le chat": 0, "cat": 0, "un café": 2, "le café": 2, "aujourd’hui": 3, "": -1, "dog": -1,
	} {
		if got := maps.FindWord(entries, word); got != want {
			t.Errorf("FindWord(%q) = %d, want %d", word, got, want)
		}
	}
}

func TestParse(t *testing.T) {
	data, err := good().Encode()
	if err != nil {
		t.Fatal(err)
	}
	m, err := maps.ParseMap(data)
	if err != nil || m.Title != "Escape Room" || len(m.Rows) != 6 || m.Locks[0].Word != "le chat" {
		t.Fatalf("parsed %+v, %v", m, err)
	}
	bad := map[string]string{
		"not json":  `{"format":`,
		"a quest":   `{"format":"hwquest","version":1}`,
		"too new":   `{"format":"hwmap","version":2}`,
		"unversion": `{"format":"hwmap"}`,
	}
	for name, s := range bad {
		if _, err := maps.ParseMap([]byte(s)); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
	if _, err := maps.ParseMap([]byte(`{"format":"hwmap","version":1,"rows":[],"x-editor":{"zoom":2}}`)); err != nil {
		t.Errorf("an unknown field is an error: %v", err)
	}
}

func TestQuest(t *testing.T) {
	a, b := good(), good()
	b.Language = "" // takes the quest's
	b.Title = "The Way Out"
	q := &maps.Quest{Title: "Week 5", Language: "fr", Intro: "Find the way out.", Ending: "Well done!", Maps: []maps.Map{*a, *b}}
	if ps := q.Check(starter(t)); len(ps) > 0 {
		t.Fatalf("problems: %v", ps)
	}
	if q.Map(1).Language != "fr" || q.Maps[1].Language != "" {
		t.Error("Map should fill in the language on a copy")
	}
	data, err := q.Encode()
	if err != nil {
		t.Fatal(err)
	}
	back, err := maps.Load("week5.HWQUEST", data)
	if err != nil || len(back.Maps) != 2 || back.Maps[1].Format != maps.MapFormat || back.Ending != "Well done!" {
		t.Fatalf("loaded %+v, %v", back, err)
	}

	q.Maps[1].Language = "la"
	q.Maps[1].Rows[1] = "#@..C."
	q.Ending = "You idiot"
	ps := q.Check(nil)
	var got []string
	for _, p := range ps {
		got = append(got, p.String())
	}
	want := []string{"ending has a word", "map 2: The map is in \"la\"", "map 2, x 0, y 1: Row 1"}
	for _, w := range want {
		if !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, w) }) {
			t.Errorf("problems %q, want one with %q", got, w)
		}
	}
	if ps := (&maps.Quest{Title: "Empty"}).Check(nil); len(ps) != 1 || !strings.Contains(ps[0].Msg, "1 to 10 maps") {
		t.Errorf("problems %v", ps)
	}

	mdata, _ := good().Encode()
	one, err := maps.Load("room.hwmap", mdata)
	if err != nil || len(one.Maps) != 1 || one.Title != "Escape Room" || one.Language != "fr" {
		t.Fatalf("single map quest %+v, %v", one, err)
	}
	if _, err := maps.Load("room.txt", mdata); err == nil {
		t.Error("loaded a .txt file")
	}
	if !maps.IsFile("a/b.HwMap") || maps.IsFile("list.txt") {
		t.Error("IsFile is wrong")
	}
	if _, err := maps.ParseQuest([]byte(`{"format":"hwquest","version":1,"maps":[{"format":"hwmap","version":9}]}`)); err == nil {
		t.Error("a quest with a too-new map parsed")
	}
}
