package maps

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/halpworld/halpwords/pkg/puzzle"
	"github.com/halpworld/halpwords/pkg/safety"
	"github.com/halpworld/halpwords/pkg/words"
)

// Problem is something wrong with a map or quest, for an editor to show.
type Problem struct {
	// Map is the map's place in a quest, from 0; 0 for a lone map. It is
	// -1 for a problem with the quest itself.
	Map int `json:"map"`
	// X and Y are the cell the problem is at, or -1 when it is about the
	// whole map.
	X   int    `json:"x"`
	Y   int    `json:"y"`
	Msg string `json:"msg"`
}

func (p Problem) String() string {
	var where []string
	if p.Map > 0 {
		where = append(where, fmt.Sprintf("map %d", p.Map+1))
	}
	if p.X >= 0 {
		where = append(where, fmt.Sprintf("x %d, y %d", p.X, p.Y))
	}
	if len(where) == 0 {
		return p.Msg
	}
	return strings.Join(where, ", ") + ": " + p.Msg
}

// checker collects a map's problems.
type checker struct {
	m        *Map
	problems []Problem
}

func (c *checker) at(x, y int, format string, args ...any) {
	c.problems = append(c.problems, Problem{X: x, Y: y, Msg: fmt.Sprintf(format, args...)})
}

func (c *checker) all(format string, args ...any) { c.at(-1, -1, format, args...) }

// Check returns what is wrong with the map, or nil when it can be played.
// It checks the grid, the things on it and the texts, and that the hero
// can reach the stairs and everything else from the start. With lists, the
// map's word lists, it also checks that each lock's word is in them and
// that its puzzle can be made from them; without, those checks wait until
// the map is played, where a word the lists lack gets a word the deck
// deals instead.
func (m *Map) Check(lists []*words.List) []Problem {
	c := &checker{m: m}
	c.texts()
	if !c.grid() {
		return c.problems
	}
	c.monsters()
	c.locks()
	c.notes()
	c.reach()
	if len(lists) > 0 {
		c.words(lists)
	}
	return c.problems
}

// text checks one of a map's or quest's texts.
func text(what, s string, most int, need bool) string {
	n := utf8.RuneCountInString(s)
	switch {
	case need && strings.TrimSpace(s) == "":
		return fmt.Sprintf("The %s is empty.", what)
	case n > most:
		return fmt.Sprintf("The %s is %d characters; the most is %d.", what, n, most)
	case !safety.Clean(s):
		return fmt.Sprintf("The %s has a word or link that is not allowed.", what)
	}
	return ""
}

func (c *checker) texts() {
	m := c.m
	if msg := text("title", m.Title, MaxTitleLen, true); msg != "" {
		c.all("%s", msg)
	}
	if _, ok := words.Lookup(m.Language); !ok && m.Language != "" {
		c.all("Unknown language %q.", m.Language)
	}
	if m.Theme != "" && m.ThemeIndex() < 0 {
		c.all("Unknown theme %q.", m.Theme)
	}
	if m.Depth < 0 || m.Depth > MaxDepth {
		c.all("The depth must be from 1 to %d.", MaxDepth)
	}
	switch m.Facing {
	case "", "N", "E", "S", "W":
	default:
		c.all("The facing must be N, E, S or W, not %q.", m.Facing)
	}
}

// grid checks the rows, and reports whether they are sound enough to check
// the rest.
func (c *checker) grid() bool {
	m := c.m
	w, h := m.Size()
	if w < MinSize || h < MinSize || w > MaxSize || h > MaxSize {
		c.all("The map is %d×%d; it must be from %d×%d to %d×%d.", w, h, MinSize, MinSize, MaxSize, MaxSize)
		return false
	}
	ok := true
	for y, r := range m.Rows {
		if len(r) != w {
			c.at(0, y, "Row %d is %d cells wide; every row must be %d.", y, len(r), w)
			ok = false
		}
		for x := 0; x < len(r); x++ {
			if !strings.ContainsRune(Cells, rune(r[x])) {
				c.at(x, y, "Unknown cell %q.", r[x])
				ok = false
			}
		}
	}
	if !ok {
		return false
	}
	starts, stairs := 0, 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cell := m.At(x, y)
			edge := x == 0 || y == 0 || x == w-1 || y == h-1
			switch {
			case edge && !Walled(cell):
				c.at(x, y, "The edge of the map must be wall.")
			case cell == Start:
				starts++
			case cell == Stairs:
				stairs++
			case cell == Door || cell == Sealed:
				if !(Walled(m.At(x-1, y)) && Walled(m.At(x+1, y))) && !(Walled(m.At(x, y-1)) && Walled(m.At(x, y+1))) {
					c.at(x, y, "A door needs walls on two opposite sides.")
				}
			}
		}
	}
	if starts != 1 {
		c.all("The map needs one start (@); it has %d.", starts)
	}
	if stairs != 1 {
		c.all("The map needs one stairs (>); it has %d.", stairs)
	}
	return starts == 1 && stairs == 1
}

func (c *checker) monsters() {
	m := c.m
	if len(m.Monsters) > MaxMonsters {
		c.all("The map has %d monsters; the most is %d.", len(m.Monsters), MaxMonsters)
	}
	taken := map[[2]int]bool{}
	bosses := 0
	for _, mo := range m.Monsters {
		switch {
		case m.At(mo.X, mo.Y) != Floor:
			c.at(mo.X, mo.Y, "A monster must stand on an empty floor cell.")
		case taken[[2]int{mo.X, mo.Y}]:
			c.at(mo.X, mo.Y, "Two monsters are on one cell.")
		}
		taken[[2]int{mo.X, mo.Y}] = true
		switch {
		case slices.Contains(BossKinds, mo.Kind):
			bosses++
		case !slices.Contains(MonsterKinds, mo.Kind):
			c.at(mo.X, mo.Y, "Unknown monster %q.", mo.Kind)
		}
		if mo.Level < 0 || mo.Level > MaxDepth {
			c.at(mo.X, mo.Y, "A monster's level must be from 1 to %d.", MaxDepth)
		}
		if mo.Trait != "" && !slices.Contains(Traits, mo.Trait) {
			c.at(mo.X, mo.Y, "Unknown trait %q.", mo.Trait)
		}
	}
	if bosses > 1 {
		c.all("The map has %d bosses; the most is one.", bosses)
	}
}

func (c *checker) locks() {
	m := c.m
	taken := map[[2]int]bool{}
	for _, l := range m.Locks {
		cell := m.At(l.X, l.Y)
		if cell != Sealed && cell != Chest {
			c.at(l.X, l.Y, "A puzzle must be on a sealed door or a chest.")
			continue
		}
		if taken[[2]int{l.X, l.Y}] {
			c.at(l.X, l.Y, "Two puzzles are set on one lock.")
		}
		taken[[2]int{l.X, l.Y}] = true
		if l.Mimic && cell != Chest {
			c.at(l.X, l.Y, "Only a chest can be a Mimic.")
		}
		k, ok := PuzzleNames[l.Puzzle]
		allowed := DoorPuzzles
		if cell == Chest {
			allowed = ChestPuzzles
		}
		if l.Word != "" && m.Language == "" {
			c.at(l.X, l.Y, "A word is set, so the map needs a language.")
		}
		if l.Puzzle == "" {
			continue
		}
		switch {
		case !ok:
			c.at(l.X, l.Y, "Unknown puzzle %q.", l.Puzzle)
		case !slices.Contains(allowed, l.Puzzle):
			c.at(l.X, l.Y, "A %s cannot have this puzzle (%s).", lockName(cell), k)
		case l.Word != "" && !puzzle.OneWord(k):
			c.at(l.X, l.Y, "This puzzle (%s) uses several words, so it cannot have one set.", k)
		}
	}
}

func lockName(c Cell) string {
	if c == Chest {
		return "chest"
	}
	return "sealed door"
}

func (c *checker) notes() {
	m := c.m
	if len(m.Notes) > MaxNotes {
		c.all("The map has %d notes; the most is %d.", len(m.Notes), MaxNotes)
	}
	taken := map[[2]int]bool{}
	for _, n := range m.Notes {
		switch {
		case !Walled(m.At(n.X, n.Y)) || n.X < 0 || n.Y < 0 || n.Y >= len(m.Rows) || n.X >= len(m.Rows[n.Y]):
			c.at(n.X, n.Y, "A note must be on a wall.")
		case taken[[2]int{n.X, n.Y}]:
			c.at(n.X, n.Y, "Two notes are on one wall.")
		}
		taken[[2]int{n.X, n.Y}] = true
		if msg := text("note", n.Text, MaxNoteLen, true); msg != "" {
			c.at(n.X, n.Y, "%s", msg)
		}
	}
}

// Passable reports whether the hero can get through a cell, perhaps after
// opening a door or solving a sealed door's puzzle. Chests, shrines,
// campfires and merchants block the way for good; monsters can be fought.
func Passable(c Cell) bool {
	switch c {
	case Floor, Start, Door, Sealed, Stairs:
		return true
	}
	return false
}

// reach checks that the hero can walk from the start to the stairs and to
// everything on the map, and that the start has a way out that is not a
// sealed door: the rule generated floors follow, so the hero is never shut
// in by a puzzle at the start.
func (c *checker) reach() {
	m := c.m
	w, h := m.Size()
	var start [2]int
	for y := range h {
		for x := range w {
			if m.At(x, y) == Start {
				start = [2]int{x, y}
			}
		}
	}
	seen := map[[2]int]bool{start: true}
	queue := [][2]int{start}
	steps := [4][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, d := range steps {
			q := [2]int{p[0] + d[0], p[1] + d[1]}
			if !seen[q] && Passable(m.At(q[0], q[1])) {
				seen[q] = true
				queue = append(queue, q)
			}
		}
	}
	near := func(x, y int) bool {
		for _, d := range steps {
			if seen[[2]int{x + d[0], y + d[1]}] {
				return true
			}
		}
		return false
	}
	for y := range h {
		for x := range w {
			switch cell := m.At(x, y); cell {
			case Stairs:
				if !seen[[2]int{x, y}] {
					c.at(x, y, "The stairs cannot be reached from the start.")
				}
			case Chest, Shrine, Campfire, Merchant, Door, Sealed:
				if !near(x, y) {
					c.at(x, y, "This %s cannot be reached from the start.", cellName(cell))
				}
			}
		}
	}
	for _, mo := range m.Monsters {
		if m.At(mo.X, mo.Y) == Floor && !seen[[2]int{mo.X, mo.Y}] {
			c.at(mo.X, mo.Y, "This monster cannot be reached from the start.")
		}
	}
	out := false
	for _, d := range steps {
		switch m.At(start[0]+d[0], start[1]+d[1]) {
		case Floor, Door, Stairs:
			out = true
		}
	}
	if !out {
		c.at(start[0], start[1], "The start needs a way out that is not a sealed door.")
	}
}

func cellName(c Cell) string {
	switch c {
	case Chest:
		return "chest"
	case Shrine:
		return "shrine"
	case Campfire:
		return "campfire"
	case Merchant:
		return "merchant"
	case Door:
		return "door"
	case Sealed:
		return "sealed door"
	}
	return "cell"
}

// tries is how many puzzles the word checks make before deciding that the
// lists cannot make a kind: some kinds need a lucky deal.
const tries = 20

// words checks each lock's word and puzzle against the lists.
func (c *checker) words(all []*words.List) {
	m := c.m
	lang, ok := words.Lookup(m.Language)
	if !ok {
		return
	}
	var lists []*words.List
	var entries []words.Entry
	for _, l := range all {
		if l.Language == m.Language {
			lists = append(lists, l)
			entries = append(entries, l.Entries...)
		}
	}
	if len(entries) == 0 {
		c.all("There are no %s words in the word list.", lang.Name)
		return
	}
	gen := puzzle.FromLists(lists)
	rng := rand.New(rand.NewPCG(1, 2))
	deck := words.NewDeck(entries, rng)
	for _, l := range m.Locks {
		cell := m.At(l.X, l.Y)
		if cell != Sealed && cell != Chest {
			continue
		}
		lock := puzzle.Door
		if cell == Chest {
			lock = puzzle.Chest
		}
		k, named := PuzzleNames[l.Puzzle]
		id := -1
		if l.Word != "" {
			if id = FindWord(entries, l.Word); id < 0 {
				c.at(l.X, l.Y, "“%s” is not in the word list.", l.Word)
				continue
			}
		}
		switch {
		case !named:
		case id >= 0 && puzzle.OneWord(k):
			if !made(func() puzzle.Puzzle {
				return puzzle.MakeWord(k, lock, m.Level(), deck, id, lang, lang.Defaults, rng, gen)
			}, k) {
				c.at(l.X, l.Y, "“%s” cannot be used for this puzzle (%s).", l.Word, k)
			}
		case id < 0:
			if !made(func() puzzle.Puzzle {
				return puzzle.MakeWith(k, lock, m.Level(), deck, lang, lang.Defaults, rng, gen)
			}, k) {
				c.at(l.X, l.Y, "The word list cannot make this puzzle (%s).", k)
			}
		}
	}
}

// made reports whether mk gives a puzzle of kind k in a few tries.
func made(mk func() puzzle.Puzzle, k puzzle.Kind) bool {
	for range tries {
		if p := mk(); p != nil && p.Kind() == k {
			return true
		}
	}
	return false
}

// FindWord returns the index in entries of the word a map names, or -1. The
// word can be any of an entry's answers, or its English; case, spacing and
// Unicode composition do not matter. Answers are matched first.
func FindWord(entries []words.Entry, word string) int {
	w := fold(word)
	if w == "" {
		return -1
	}
	for i, e := range entries {
		for _, a := range e.Answers {
			if fold(a) == w {
				return i
			}
		}
	}
	for i, e := range entries {
		if fold(e.Prompt) == w {
			return i
		}
	}
	return -1
}

func fold(s string) string {
	s = strings.Join(strings.Fields(norm.NFC.String(s)), " ")
	s = strings.ReplaceAll(s, "’", "'")
	return strings.ToLower(s)
}

// Check returns what is wrong with the quest and its maps, or nil when it
// can be played. Lists are as for Map.Check.
func (q *Quest) Check(lists []*words.List) []Problem {
	var out []Problem
	add := func(msg string) { out = append(out, Problem{Map: -1, X: -1, Y: -1, Msg: msg}) }
	if msg := text("quest's title", q.Title, MaxTitleLen, true); msg != "" {
		add(msg)
	}
	if msg := text("introduction", q.Intro, MaxTextLen, false); msg != "" {
		add(msg)
	}
	if msg := text("ending", q.Ending, MaxTextLen, false); msg != "" {
		add(msg)
	}
	if _, ok := words.Lookup(q.Language); !ok && q.Language != "" {
		add(fmt.Sprintf("Unknown language %q.", q.Language))
	}
	if len(q.Maps) == 0 || len(q.Maps) > MaxMaps {
		add(fmt.Sprintf("A quest has from 1 to %d maps; this one has %d.", MaxMaps, len(q.Maps)))
	}
	for i := range q.Maps {
		m := q.Map(i)
		if q.Maps[i].Language != "" && q.Maps[i].Language != q.Language {
			out = append(out, Problem{Map: i, X: -1, Y: -1, Msg: fmt.Sprintf("The map is in %q, the quest in %q.", m.Language, q.Language)})
		}
		for _, p := range m.Check(lists) {
			p.Map = i
			out = append(out, p)
		}
	}
	return out
}
