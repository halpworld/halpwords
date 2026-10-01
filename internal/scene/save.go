package scene

import (
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"io/fs"
	"time"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/llm"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/raycast"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/pkg/compete"
	"github.com/halpworld/halpwords/pkg/maps"
	"github.com/halpworld/halpwords/pkg/words"
)

// saveName is the file the adventure is saved in. There is one save slot.
const saveName = "adventure.json"

// saveVersion changes when older saves can no longer be loaded.
const saveVersion = 2

// saveFile is a saved adventure. Continue goes to Suspend if there is one,
// and it is then deleted, so a suspended game can only be picked up once.
// Otherwise it goes back to the last Save Shrine. Floors come back from the
// run's seed, so only what has changed on them is kept.
type saveFile struct {
	Version    int
	Language   string // a words.Language code
	Class      rpg.Class
	Seed       uint64
	RNG        []byte // the run's random generator
	Greek      bool
	SeenTraits dungeon.Trait
	Deck       words.DeckState
	Perfect    []int `json:",omitempty"` // words spelled perfectly
	Log        []savedLine
	Shrine     checkpoint
	Suspend    *checkpoint `json:",omitempty"`
	// Mode is Adventure when left out, as in saves from before Hardcore.
	Mode  compete.Mode  `json:",omitempty"`
	Day   string        `json:",omitempty"` // a Daily Dungeon's date
	Tally compete.Tally // what a Hardcore score counts
	// Played is how long the run has been played, in seconds, for the
	// rankings' plausibility check.
	Played float64 `json:",omitempty"`
	// Scripts are the Dungeon Director's scripts for the floors the save
	// can go back to, by depth.
	Scripts map[int]*llm.Script `json:",omitempty"`
	// Quest is the hand-made quest being played, whole, so the save does
	// not depend on the quest file staying where it was.
	Quest *maps.Quest `json:",omitempty"`
	// Assignment is the assignment quest the adventure plays, if it was
	// started for one. Its list is not kept: it comes from the lists the
	// game has.
	Assignment *assignRun `json:",omitempty"`
}

type savedLine struct {
	Text string
	Col  color.RGBA
}

// encodeSave saves the run. With suspend, it also keeps the game as it is
// now, with the hero at at facing facing on floor l.
func encodeSave(r *run, l *dungeon.Level, at dungeon.Point, facing dungeon.Dir, suspend bool) ([]byte, error) {
	rng, err := r.src.MarshalBinary()
	if err != nil {
		return nil, err
	}
	s := saveFile{
		Version:    saveVersion,
		Language:   r.lang.Code,
		Class:      r.hero.Class,
		Seed:       r.seed,
		RNG:        rng,
		Greek:      r.greek,
		SeenTraits: r.seenTraits,
		Deck:       r.deck.State(),
		Shrine:     r.shrine,
		Mode:       r.mode,
		Day:        r.day,
		Tally:      r.tally,
		Played:     r.played,
		Quest:      r.quest,
		Assignment: r.assign,
	}
	if r.ai != nil {
		for depth, sc := range r.ai.scripts {
			if depth == r.depth || depth == r.shrine.Depth {
				if s.Scripts == nil {
					s.Scripts = map[int]*llm.Script{}
				}
				s.Scripts[depth] = sc
			}
		}
	}
	for id := range r.deck.Entries() {
		if r.perfect[id] {
			s.Perfect = append(s.Perfect, id)
		}
	}
	if suspend {
		cp := r.here(l, at, facing)
		s.Suspend = &cp
	}
	for _, line := range r.log {
		s.Log = append(s.Log, savedLine{line.text, line.col})
	}
	return json.Marshal(s)
}

// loaded is a run and floor rebuilt from a save.
type loaded struct {
	run       *run
	level     *dungeon.Level
	at        dungeon.Point
	facing    dungeon.Dir
	suspended bool // from a suspended game, not a shrine
}

var errDamaged = errors.New("the save file is damaged")

// decodeSave rebuilds a saved run and the floor it was on.
func decodeSave(ctx *game.Context, data []byte) (*loaded, error) {
	var s saveFile
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, errDamaged
	}
	if s.Version != saveVersion {
		return nil, fmt.Errorf("the save is from another version of the game")
	}
	lang, ok := words.Lookup(s.Language)
	if !ok || len(ctx.ListsFor(lang.Code)) == 0 {
		return nil, fmt.Errorf("no word lists for %s", s.Language)
	}
	if s.Mode.Scored() && s.Suspend == nil {
		// Hardcore runs are only saved when suspended, and the save is
		// deleted as it is loaded.
		return nil, errors.New("that Hardcore run is over")
	}
	if s.Quest != nil && len(s.Quest.Maps) == 0 {
		return nil, errDamaged
	}
	if _, ok := runLists(ctx, lang, s.Quest, s.Assignment); !ok {
		return nil, fmt.Errorf("the word list for the %s assignment is gone", s.Assignment.Title)
	}
	r := beginRun(ctx, lang, s.Class, s.Seed, s.Quest, s.Assignment)
	if err := r.src.UnmarshalBinary(s.RNG); err != nil {
		return nil, errDamaged
	}
	r.setMode(ctx, s.Mode)
	r.day, r.tally, r.played = s.Day, s.Tally, s.Played
	r.greek = s.Greek
	r.seenTraits = s.SeenTraits
	r.deck.SetState(s.Deck)
	for _, id := range s.Perfect {
		r.perfect[id] = true
	}
	for _, line := range s.Log {
		r.log = append(r.log, logLine{line.Text, line.Col})
	}
	r.shrine = s.Shrine
	r.onDisk = true
	for depth, sc := range s.Scripts {
		if sc != nil {
			r.ai.scripts[depth] = sc
		}
	}

	cp := s.Shrine
	if s.Suspend != nil {
		cp = *s.Suspend
	}
	l, at, facing, err := r.enter(cp)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errDamaged, err)
	}
	if r.shrine.Depth < 1 {
		return nil, errDamaged
	}
	return &loaded{run: r, level: l, at: at, facing: facing, suspended: s.Suspend != nil}, nil
}

// saveSummary describes the saved adventure for the title screen, such as
// "French · Knight · Floor 3". ok is false when there is no save.
func saveSummary() (summary string, ok bool) {
	data, err := save.Read(saveName)
	if err != nil {
		// Only a missing file is no save: one that can't be read is
		// still there, and Continue will explain.
		return "", !errors.Is(err, fs.ErrNotExist)
	}
	var s struct {
		Language   string
		Class      rpg.Class
		Shrine     struct{ Depth int }
		Suspend    *struct{ Depth int }
		Mode       compete.Mode
		Quest      *struct{ Title string }
		Assignment *struct{ Title string }
	}
	if json.Unmarshal(data, &s) != nil {
		return "", true // Continue will explain the problem
	}
	name := s.Language
	if l, ok := words.Lookup(s.Language); ok {
		name = l.Name
	}
	depth := s.Shrine.Depth
	if s.Suspend != nil {
		depth = s.Suspend.Depth
	}
	summary = fmt.Sprintf("%s · %s · Floor %d", name, s.Class, depth)
	if s.Mode != compete.Adventure {
		summary += " · " + s.Mode.String()
	}
	if s.Quest != nil {
		summary = fmt.Sprintf("%s · %s · %s · Floor %d", s.Quest.Title, name, s.Class, depth)
	}
	if s.Assignment != nil {
		summary += " · Assignment"
	}
	return summary, true
}

// savedHero names the hero in the save slot, such as "Knight", and the
// floor they are on. ok is false when there is no save. A save that can't
// be read still counts, with no hero named: starting a run would replace it.
func savedHero() (hero string, floor int, ok bool) {
	data, err := save.Read(saveName)
	if err != nil {
		return "", 0, !errors.Is(err, fs.ErrNotExist)
	}
	var s struct {
		Class   rpg.Class
		Shrine  struct{ Depth int }
		Suspend *struct{ Depth int }
	}
	if json.Unmarshal(data, &s) != nil {
		return "", 0, true
	}
	floor = s.Shrine.Depth
	if s.Suspend != nil {
		floor = s.Suspend.Depth
	}
	return s.Class.String(), floor, true
}

// loadCrawl resumes the saved adventure. A suspended game is deleted from
// the save as it is loaded.
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
	switch {
	case s.run.hardcore():
		// A Hardcore run can be picked up once: until it is suspended
		// again, it is only in memory.
		if err := save.Remove(saveName); err != nil {
			return nil, fmt.Errorf("could not update the save")
		}
		s.run.onDisk = false
		c.showBanner("Welcome back!", fmt.Sprintf("Floor %d · %s", s.run.depth, c.floorName()))
		s.run.say("Welcome back! Your Hardcore run continues.", pal.Yellow)
		return c, nil
	case s.suspended:
		if !c.writeSave(ctx, false) {
			return nil, fmt.Errorf("could not update the save")
		}
		c.showBanner("Welcome back!", fmt.Sprintf("Floor %d · %s", s.run.depth, c.floorName()))
		s.run.say("Welcome back! Your adventure continues.", pal.Yellow)
	default:
		c.showBanner("Welcome back!", "You wake at the shrine")
		s.run.say(fmt.Sprintf("Welcome back! You wake at the shrine on floor %d.", s.run.depth), pal.Yellow)
	}
	c.lastSave, _ = encodeSave(c.run, c.level, c.pos, c.facing, true)
	c.unsaved = false
	return c, nil
}

// writeSave writes the adventure to the save slot, replacing any older
// save. With suspend, the game as it is now is kept too.
func (c *Crawl) writeSave(ctx *game.Context, suspend bool) bool {
	data, err := encodeSave(c.run, c.level, c.pos, c.facing, suspend)
	if err == nil {
		err = save.Write(saveName, data)
	}
	if err != nil {
		ctx.Notify("Could not save")
		c.run.say("Could not save the game: "+err.Error(), pal.Rose)
		return false
	}
	c.run.onDisk = true
	c.lastSave, _ = encodeSave(c.run, c.level, c.pos, c.facing, true)
	c.unsaved = false
	return true
}

// OnClose implements game.Closer: the window or the page is closing, so a
// run that Suspend would keep is written the way Suspend writes it (#49).
// An Adventure comes back where it was; a Hardcore or Daily run is a
// suspend too, and so can be picked up once, like any suspend.
//
// Nothing is written where the pause menu would not offer Suspend: in a
// battle or a puzzle, after a fall, or in a race. The save already on disk
// is then left as it is.
func (c *Crawl) OnClose(ctx *game.Context) {
	if !c.canSuspend() {
		return
	}
	if c.writeSave(ctx, true) && c.run.hardcore() {
		c.closeSaved, c.closedAt = true, time.Now()
	}
}

// canSuspend reports whether the game could be suspended now: the pause
// menu offers Suspend when the hero is exploring and it is not a race.
func (c *Crawl) canSuspend() bool {
	m := c.mode
	if m == modePause || m == modeQuit {
		m = c.resume
	}
	return m == modeExplore && c.run.race == nil
}

// closeGrace is how long after OnClose the game must still be running
// for it to count as going on: a web page may run one more frame after it
// is hidden, and must not take its own save back then.
const closeGrace = time.Second

// afterClose is called every update, and does something when the game goes
// on after OnClose, as a web page does when it is shown again. A Hardcore
// run's suspend is taken back off the disk, so the run is in memory only
// again: it can be picked up once, and a crash after that still loses it.
func (c *Crawl) afterClose(now time.Time) {
	if !c.closeSaved || now.Sub(c.closedAt) < closeGrace {
		return
	}
	c.closeSaved = false
	if c.run.hardcore() && c.run.onDisk {
		save.Remove(saveName)
		c.run.onDisk = false
	}
}

// savedScored reports whether the save slot holds a Hardcore or Daily run,
// which can't be got back once it is replaced.
func savedScored() bool {
	data, err := save.Read(saveName)
	if err != nil {
		return false
	}
	var s struct{ Mode compete.Mode }
	return json.Unmarshal(data, &s) == nil && s.Mode.Scored()
}
