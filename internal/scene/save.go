package scene

import (
	"encoding/json"
	"fmt"
	"image/color"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/raycast"
	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/internal/words"
)

// saveName is the file the adventure is saved in. There is one save slot.
const saveName = "adventure.json"

// saveVersion changes when older saves can no longer be loaded.
const saveVersion = 1

// saveFile is a saved adventure: the run, and the floor the hero is on.
// The floor itself comes back from the run's seed, so only what has changed
// on it is kept.
type saveFile struct {
	Version    int
	Language   string // a words.Language code
	Seed       uint64
	RNG        []byte // the run's random generator
	Depth      int
	Hero       hero
	Checkpoint hero // the hero as they arrived on the floor
	Greek      bool
	SeenTraits dungeon.Trait
	Deck       words.DeckState
	Log        []savedLine
	At         dungeon.Point
	Facing     dungeon.Dir
	Floor      dungeon.State
}

type savedLine struct {
	Text string
	Col  color.RGBA
}

// encodeSave saves the run with the hero at at, facing facing, on floor l.
func encodeSave(r *run, l *dungeon.Level, at dungeon.Point, facing dungeon.Dir) ([]byte, error) {
	rng, err := r.src.MarshalBinary()
	if err != nil {
		return nil, err
	}
	s := saveFile{
		Version:    saveVersion,
		Language:   r.lang.Code,
		Seed:       r.seed,
		RNG:        rng,
		Depth:      r.depth,
		Hero:       r.hero,
		Checkpoint: r.saved,
		Greek:      r.greek,
		SeenTraits: r.seenTraits,
		Deck:       r.deck.State(),
		At:         at,
		Facing:     facing,
		Floor:      l.State(),
	}
	for _, line := range r.log {
		s.Log = append(s.Log, savedLine{line.text, line.col})
	}
	return json.Marshal(s)
}

// loaded is a run and floor rebuilt from a save.
type loaded struct {
	run    *run
	level  *dungeon.Level
	at     dungeon.Point
	facing dungeon.Dir
}

// decodeSave rebuilds a saved run and the floor it was on.
func decodeSave(ctx *game.Context, data []byte) (*loaded, error) {
	var s saveFile
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("the save file is damaged")
	}
	if s.Version != saveVersion {
		return nil, fmt.Errorf("the save is from another version of the game")
	}
	lang, ok := words.Lookup(s.Language)
	if !ok || len(ctx.ListsFor(lang.Code)) == 0 {
		return nil, fmt.Errorf("no word lists for %s", s.Language)
	}
	if s.Depth < 1 {
		return nil, fmt.Errorf("the save file is damaged")
	}
	r := startRun(ctx, lang, s.Seed)
	if err := r.src.UnmarshalBinary(s.RNG); err != nil {
		return nil, fmt.Errorf("the save file is damaged")
	}
	r.depth = s.Depth
	r.hero, r.saved = s.Hero, s.Checkpoint
	r.greek = s.Greek
	r.seenTraits = s.SeenTraits
	r.deck.SetState(s.Deck)
	for _, line := range s.Log {
		r.log = append(r.log, logLine{line.Text, line.Col})
	}

	l := dungeon.Generate(r.floorSeed(r.depth), r.depth)
	if err := l.Restore(s.Floor); err != nil {
		return nil, fmt.Errorf("the save file is damaged: %w", err)
	}
	if !l.At(s.At).Walkable() {
		return nil, fmt.Errorf("the save file is damaged")
	}
	return &loaded{run: r, level: l, at: s.At, facing: s.Facing & 3}, nil
}

// saveSummary describes the saved adventure for the title screen, such as
// "French · Floor 3". ok is false when there is no save.
func saveSummary() (summary string, ok bool) {
	data, err := save.Read(saveName)
	if err != nil {
		return "", false
	}
	var s struct {
		Language string
		Depth    int
	}
	if json.Unmarshal(data, &s) != nil {
		return "", true // Continue will explain the problem
	}
	name := s.Language
	if l, ok := words.Lookup(s.Language); ok {
		name = l.Name
	}
	return fmt.Sprintf("%s · Floor %d", name, s.Depth), true
}

// loadCrawl resumes the saved adventure where it was saved.
func loadCrawl(ctx *game.Context) (*Crawl, error) {
	data, err := save.Read(saveName)
	if err != nil {
		return nil, err
	}
	s, err := decodeSave(ctx, data)
	if err != nil {
		return nil, err
	}
	c := crawlOn(s.run, s.level)
	c.pos, c.facing, c.angle = s.at, s.facing, raycast.Angle(s.facing)
	c.showBanner("Welcome back!", fmt.Sprintf("Floor %d · %s", s.run.depth, c.theme.Name))
	s.run.say("Welcome back! Your adventure continues.", pal.Yellow)
	c.lastSave, _ = encodeSave(c.run, c.level, c.pos, c.facing)
	return c, nil
}
