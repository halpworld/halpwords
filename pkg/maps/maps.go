// Package maps is the format of hand-made dungeon floors (.hwmap) and
// quests (.hwquest), and the checks a map must pass to be played. The game
// builds a floor from a map with dungeon.FromMap; halpwords-server's map
// editor uses the same checks, so the two never disagree. It has no
// Ebitengine dependency.
//
// A map is JSON. Its grid is a list of rows, one character per cell:
//
//	#  wall             .  floor            @  start (floor)
//	T  wall with torch  +  door             =  sealed door
//	C  chest            >  stairs down      S  Save Shrine
//	F  campfire         M  merchant
//
// Monsters, the puzzles on sealed doors and chests, and notes on walls are
// listed separately, by cell.
package maps

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/halpworld/halpwords/pkg/puzzle"
)

// Format names and the newest version of each this package reads. A file
// with a newer version is refused, so an old game never plays a map it
// would get wrong.
const (
	MapFormat   = "hwmap"
	QuestFormat = "hwquest"
	Version     = 1
)

// Limits on a map's size and contents.
const (
	MinSize     = 5   // cells across and down, walls included
	MaxSize     = 40  // the biggest generated floor
	MaxDepth    = 30  // the deepest floor a map can play as
	MaxMonsters = 60  // monsters on one map
	MaxNotes    = 40  // notes on one map
	MaxNoteLen  = 140 // characters in a note
	MaxTitleLen = 60  // characters in a map or quest title
	MaxTextLen  = 1000
	MaxMaps     = 10 // maps in a quest
)

// Cell is one character of a map's grid.
type Cell = byte

// The cells of a map's grid.
const (
	Wall     Cell = '#'
	Torch    Cell = 'T' // a wall with a torch on it
	Floor    Cell = '.'
	Start    Cell = '@' // the floor the hero starts on
	Door     Cell = '+'
	Sealed   Cell = '=' // a rune-sealed door: a word puzzle opens it
	Chest    Cell = 'C'
	Stairs   Cell = '>'
	Shrine   Cell = 'S'
	Campfire Cell = 'F'
	Merchant Cell = 'M'
)

// Cells lists every cell character, for editors.
const Cells = "#T.@+=C>SFM"

// Map is one hand-made floor.
type Map struct {
	Format  string `json:"format"`  // MapFormat
	Version int    `json:"version"` // 1
	Title   string `json:"title"`
	// Language is the code of the language the map's words are in, such
	// as "fr". Empty means any: the player chooses, and no lock can name
	// a word.
	Language string   `json:"language"`
	List     *ListRef `json:"list,omitempty"`
	// Theme is one of Themes, such as "Ice Halls". Empty means the usual
	// look for Depth.
	Theme string `json:"theme,omitempty"`
	// Depth is the generated floor the map plays like: how strong its
	// monsters are by default, which puzzles and how hard, and what
	// chests hold. 0 means 1.
	Depth int `json:"depth,omitempty"`
	// Facing is the way the hero faces at the start: "N", "E", "S" or
	// "W". Empty means the most open way.
	Facing   string    `json:"facing,omitempty"`
	Rows     []string  `json:"rows"`
	Monsters []Monster `json:"monsters,omitempty"`
	Locks    []Lock    `json:"locks,omitempty"`
	Notes    []Note    `json:"notes,omitempty"`
}

// ListRef names the word list a map's words come from. A game that has
// the list (by ID) deals words only from it; otherwise from every list in
// the map's language.
type ListRef struct {
	ID      string `json:"id,omitempty"` // the server's list ID
	Version int    `json:"version,omitempty"`
	Title   string `json:"title,omitempty"`
}

// Monster is a monster placed on a floor cell.
type Monster struct {
	X    int    `json:"x"`
	Y    int    `json:"y"`
	Kind string `json:"kind"` // a name in MonsterKinds or BossKinds
	// Level is the floor depth its HP and attack are scaled for. 0 means
	// the map's depth.
	Level int `json:"level,omitempty"`
	// Trait is an extra power, one of Traits, or empty.
	Trait string `json:"trait,omitempty"`
}

// Lock sets the puzzle on a sealed door or a chest. Sealed doors and
// chests without one get a random puzzle, as on a generated floor.
type Lock struct {
	X int `json:"x"`
	Y int `json:"y"`
	// Puzzle is one of PuzzleNames, or empty for a random one.
	Puzzle string `json:"puzzle,omitempty"`
	// Word is the word the puzzle is about: an answer or the English of
	// a word in the list. Only for puzzles about one word; empty means
	// the deck deals one.
	Word string `json:"word,omitempty"`
	// Mimic makes a chest a Mimic, which bites when its puzzle fails.
	Mimic bool `json:"mimic,omitempty"`
}

// Note is text written on a wall, read when the hero faces it.
type Note struct {
	X    int    `json:"x"`
	Y    int    `json:"y"`
	Text string `json:"text"`
}

// MonsterKinds are the monsters a map can place, weakest first. They are
// the game's monster kinds (the dungeon package's tests keep the two the
// same).
var MonsterKinds = []string{
	"Green Slime", "Cave Bat", "Grumpy Rat", "Bone Rattler", "Wisp", "Gazer",
	"Crypt Spider", "Mirror Imp", "Blue Slime", "Moss Golem", "Fire Imp",
}

// BossKinds are the bosses a map can place. A boss holds the stairs shut
// until it is defeated. A map has at most one.
var BossKinds = []string{"Slime King", "Bone Lord", "Gazer Queen", "Golem Titan", "Imp Overlord"}

// Traits are the extra powers a monster can have.
var Traits = []string{"armored", "ghostly", "mirrored", "swift"}

// PuzzleNames maps the puzzle names used in maps to puzzle kinds.
var PuzzleNames = map[string]puzzle.Kind{
	"reverse":     puzzle.Reverse,
	"odd-one-out": puzzle.OddOneOut,
	"anagram":     puzzle.Anagram,
	"missing":     puzzle.Missing,
	"spell":       puzzle.Spell,
	"pairs":       puzzle.Pairs,
	"riddle":      puzzle.Riddle,
	"tumbler":     puzzle.Tumbler,
	"crossword":   puzzle.Crossword,
	"gap-fill":    puzzle.Cloze,
}

// DoorPuzzles and ChestPuzzles are the puzzles each lock can have, as on
// generated floors, plus gap-fill sentences for both.
var (
	DoorPuzzles  = []string{"reverse", "anagram", "odd-one-out", "pairs", "riddle", "spell", "gap-fill"}
	ChestPuzzles = []string{"missing", "anagram", "tumbler", "spell", "crossword", "gap-fill"}
)

// ParseMap reads a .hwmap file. It checks only that it is a map this
// version can read; Check finds everything else.
func ParseMap(data []byte) (*Map, error) {
	var m Map
	if err := decode(data, &m); err != nil {
		return nil, err
	}
	if err := checkFormat(m.Format, MapFormat, m.Version); err != nil {
		return nil, err
	}
	return &m, nil
}

// Encode writes m as indented JSON, with its format and version set.
func (m *Map) Encode() ([]byte, error) {
	c := *m
	c.Format, c.Version = MapFormat, Version
	return json.MarshalIndent(&c, "", "  ")
}

func decode(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("not a map file: %v", err)
	}
	return nil
}

func checkFormat(got, want string, version int) error {
	switch {
	case got != want:
		return fmt.Errorf("not a .%s file (format %q)", want, got)
	case version < 1:
		return fmt.Errorf("the .%s file has no version", want)
	case version > Version:
		return fmt.Errorf("the .%s file is version %d; this game reads up to %d, so it needs updating", want, version, Version)
	}
	return nil
}

// Size returns the map's width and height. The width is the longest row.
func (m *Map) Size() (w, h int) {
	for _, r := range m.Rows {
		w = max(w, len(r))
	}
	return w, len(m.Rows)
}

// At returns the cell at x, y. Cells off the grid are walls.
func (m *Map) At(x, y int) Cell {
	if y < 0 || y >= len(m.Rows) || x < 0 || x >= len(m.Rows[y]) {
		return Wall
	}
	return m.Rows[y][x]
}

// Level returns the floor depth the map plays like: Depth, or 1.
func (m *Map) Level() int { return max(1, m.Depth) }

// Themes are the floor looks a map can have, in the order of the game's
// proc.Themes (the tests keep the two the same).
var Themes = []string{"The Crypt", "Mossy Cellars", "Flooded Caves", "Ice Halls", "Lava Forge", "Amethyst Vaults"}

// ThemeIndex returns the index in Themes of the map's theme, ignoring
// case, or -1 when it has none: the floor then has the usual look for its
// depth.
func (m *Map) ThemeIndex() int {
	for i, t := range Themes {
		if strings.EqualFold(t, m.Theme) {
			return i
		}
	}
	return -1
}

// Walled reports whether c is a wall, with or without a torch.
func Walled(c Cell) bool { return c == Wall || c == Torch }
