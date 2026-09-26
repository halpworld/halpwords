package maps

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

// Quest is 1 to MaxMaps maps played in order, with a text before the
// first and after the last. Taking the stairs on the last map ends it.
type Quest struct {
	Format  string `json:"format"`  // QuestFormat
	Version int    `json:"version"` // 1
	Title   string `json:"title"`
	// Language is every map's language (maps may leave theirs empty).
	// Empty means any: the player chooses.
	Language string `json:"language"`
	Intro    string `json:"intro,omitempty"`
	Ending   string `json:"ending,omitempty"`
	Maps     []Map  `json:"maps"`
}

// ParseQuest reads a .hwquest file. It checks only that it is a quest this
// version can read; Check finds everything else.
func ParseQuest(data []byte) (*Quest, error) {
	var q Quest
	if err := decode(data, &q); err != nil {
		return nil, err
	}
	if err := checkFormat(q.Format, QuestFormat, q.Version); err != nil {
		return nil, err
	}
	for i := range q.Maps {
		if err := checkFormat(q.Maps[i].Format, MapFormat, q.Maps[i].Version); err != nil {
			return nil, fmt.Errorf("map %d: %v", i+1, err)
		}
	}
	return &q, nil
}

// Load reads a .hwquest file, or a .hwmap file as a quest of one map, by
// the file's name. Other names are an error.
func Load(name string, data []byte) (*Quest, error) {
	switch strings.ToLower(path.Ext(name)) {
	case "." + QuestFormat:
		return ParseQuest(data)
	case "." + MapFormat:
		m, err := ParseMap(data)
		if err != nil {
			return nil, err
		}
		return Single(m), nil
	}
	return nil, fmt.Errorf("%s is not a .hwmap or .hwquest file", name)
}

// IsFile reports whether name is a map or quest file, by its extension.
func IsFile(name string) bool {
	ext := strings.ToLower(path.Ext(name))
	return ext == "."+MapFormat || ext == "."+QuestFormat
}

// Map returns a copy of map i as it is played: in the quest's language
// when it names none.
func (q *Quest) Map(i int) *Map {
	m := q.Maps[i]
	if m.Language == "" {
		m.Language = q.Language
	}
	return &m
}

// Single makes a quest of one map, with the map's title and language.
func Single(m *Map) *Quest {
	return &Quest{Format: QuestFormat, Version: Version, Title: m.Title, Language: m.Language, Maps: []Map{*m}}
}

// Encode writes q as indented JSON, with its and its maps' formats and
// versions set.
func (q *Quest) Encode() ([]byte, error) {
	c := *q
	c.Format, c.Version = QuestFormat, Version
	c.Maps = append([]Map(nil), q.Maps...)
	for i := range c.Maps {
		c.Maps[i].Format, c.Maps[i].Version = MapFormat, Version
	}
	return json.MarshalIndent(&c, "", "  ")
}
