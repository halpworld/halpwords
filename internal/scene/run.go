package scene

import (
	"fmt"
	"image/color"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/profile"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/pkg/compete"
	"github.com/halpworld/halpwords/pkg/maps"
	"github.com/halpworld/halpwords/pkg/proc"
	"github.com/halpworld/halpwords/pkg/puzzle"
	"github.com/halpworld/halpwords/pkg/words"
)

// hero is the player character.
type hero = rpg.Hero

// checkpoint is a point in an adventure to go back to: the last Save
// Shrine used, or a suspended game.
type checkpoint struct {
	Depth int
	Regen int // how many times floors had been remade
	Hero  hero
	// Floor, At and Facing are the floor as it was and where the hero
	// stood. Without a floor, the floor is new and the hero at its start.
	Floor  *dungeon.State `json:",omitempty"`
	At     dungeon.Point
	Facing dungeon.Dir
}

// logLine is one message in the crawl's message log.
type logLine struct {
	text string
	col  color.RGBA
}

// run is one adventure: everything that carries over from floor to floor.
type run struct {
	lang *words.Language
	deck *words.Deck
	rng  *rand.Rand
	src  *rand.PCG // rng's generator, kept so saves can store its state
	seed uint64

	hero hero
	// shrine is where the hero wakes up after falling: the last Save
	// Shrine they used, or the start of the adventure.
	shrine checkpoint
	depth  int
	// regen counts the times the hero has fallen. It changes the floors,
	// so a floor is new after waking at a shrine.
	regen int
	log   []logLine
	greek bool // Greek keys on, for Greek

	// perfect marks the words the hero has spelled perfectly on this
	// adventure. The first perfect spelling of a word is worth XP.
	perfect map[int]bool
	// onDisk is set once this adventure is in the save file, so falling
	// can update it there.
	onDisk bool

	// seenTraits are the monster traits the hero has been told about.
	seenTraits dungeon.Trait
	sound      *game.Sound

	// mode is Adventure, Hardcore or the Daily Dungeon.
	mode compete.Mode
	// day is the date of a Daily Dungeon, such as "2026-09-24".
	day string
	// tally counts what the hero has done, for a Hardcore score.
	tally compete.Tally
	// settings are the grading rules and timers. Hardcore runs always use
	// the language's preset.
	settings profile.LangSettings
	prof     *profile.Profile
	// link sends answers to a grown-up's account, when the game is linked.
	link *link.Client
	// ai is what the run asks the AI for, when one is set up.
	ai *runAI
	// cloze are the gap-fill sentences written in the word lists, or nil.
	cloze *puzzle.Generated
	// quest is the hand-made quest being played, or nil for the usual
	// dungeon. Its maps are the floors, in order.
	quest *maps.Quest
}

// runSetup is what the New Adventure screens choose before the class.
type runSetup struct {
	mode compete.Mode
	// seed is the dungeon for a seed challenge or a Daily Dungeon; when
	// seeded is false a new one is made.
	seed   uint64
	seeded bool
	day    string // the Daily Dungeon's date
	quest  *maps.Quest
}

// dailySetup is today's Daily Dungeon in lang.
func dailySetup(ctx *game.Context, lang *words.Language) runSetup {
	now := time.Now()
	return runSetup{mode: compete.Daily, seed: compete.DailySeed(now, lang.Code, entriesFor(ctx, lang)), seeded: true, day: now.Format(time.DateOnly)}
}

// entriesFor returns every word in the lists for lang.
func entriesFor(ctx *game.Context, lang *words.Language) []words.Entry {
	var entries []words.Entry
	for _, l := range ctx.ListsFor(lang.Code) {
		entries = append(entries, l.Entries...)
	}
	return entries
}

func newRun(ctx *game.Context, lang *words.Language, class rpg.Class, setup runSetup) *run {
	seed := setup.seed
	if !setup.seeded {
		seed = compete.RandomSeed(proc.NewRand(uint64(time.Now().UnixNano())))
		// HALPWORDS_SEED replays a dungeon, for testing and bug reports.
		if v, err := strconv.ParseUint(os.Getenv("HALPWORDS_SEED"), 10, 64); err == nil {
			seed = v
		}
	}
	r := startRun(ctx, lang, class, seed, setup.quest)
	r.setMode(ctx, setup.mode)
	r.day = setup.day
	return r
}

// setMode sets how the run is played, and the settings that go with it.
func (r *run) setMode(ctx *game.Context, m compete.Mode) {
	r.mode = m
	r.settings = profile.Preset(r.lang)
	if !m.Scored() && ctx.Profile != nil {
		r.settings = ctx.Profile.Settings.For(r.lang)
	}
}

// questLists returns the word lists a run in lang deals from: for a quest
// whose maps name lists the game has (by ID), those; otherwise every list
// for lang.
func questLists(ctx *game.Context, lang *words.Language, q *maps.Quest) []*words.List {
	all := ctx.ListsFor(lang.Code)
	if q == nil {
		return all
	}
	var named []*words.List
	for _, l := range all {
		for _, m := range q.Maps {
			if m.List != nil && m.List.ID != "" && m.List.ID == l.ID && !slices.Contains(named, l) {
				named = append(named, l)
			}
		}
	}
	if len(named) == 0 {
		return all
	}
	return named
}

// startRun begins an Adventure in lang as a hero of class through the
// dungeon made from seed, or through quest when it is not nil.
func startRun(ctx *game.Context, lang *words.Language, class rpg.Class, seed uint64, quest *maps.Quest) *run {
	lists := questLists(ctx, lang, quest)
	var entries []words.Entry
	for _, l := range lists {
		entries = append(entries, l.Entries...)
	}
	src := proc.NewPCG(seed)
	rng := rand.New(src)
	r := &run{
		lang:    lang,
		deck:    words.NewDeck(entries, rng),
		rng:     rng,
		src:     src,
		seed:    seed,
		hero:    rpg.NewHero(class),
		depth:   1,
		greek:   lang.Script == words.ScriptGreek,
		sound:   ctx.Sound,
		perfect: map[int]bool{},
		prof:    ctx.Profile,
		link:    ctx.Link,
		ai:      newRunAI(ctx),
		cloze:   puzzle.FromLists(lists),
		quest:   quest,
	}
	if r.prof != nil {
		r.deck.SetMemory(r.prof.MemoryFor(lang.Code))
	}
	r.setMode(ctx, compete.Adventure)
	r.shrine = checkpoint{Depth: 1, Hero: r.hero.Clone()}
	return r
}

// linkMode is the run's mode as the grown-up's account knows it:
// "adventure", or "hardcore" for Hardcore and the Daily Dungeon.
func (r *run) linkMode() string {
	if r.mode.Scored() {
		return "hardcore"
	}
	return "adventure"
}

// rules are how answers are graded on this run.
func (r *run) rules() words.Rules { return r.settings.Rules }

// hardcore reports whether the run has one life and a score.
func (r *run) hardcore() bool { return r.mode.Scored() }

// score is the run's score so far.
func (r *run) score() int { return r.tally.Score(r.depth) }

// seedCode is the code that replays the run's dungeon.
func (r *run) seedCode() string { return compete.SeedCode(r.seed) }

// floor makes the map for floor depth: a quest's map, or a generated one.
// Hardcore floors have no shrines.
func (r *run) floor(depth int) *dungeon.Level {
	if m := r.questMap(depth); m != nil {
		if l, err := dungeon.FromMap(m, r.floorSeed(depth)); err == nil {
			return l
		}
		// Quests are checked before they are played, so this is a bug;
		// a generated floor keeps the game going.
	}
	l := dungeon.Generate(r.floorSeed(depth), depth)
	if r.hardcore() {
		l.Harden()
	}
	return l
}

// questMap returns the quest's map for floor depth, or nil when the run is
// not a quest.
func (r *run) questMap(depth int) *maps.Map {
	if r.quest == nil || depth < 1 || depth > len(r.quest.Maps) {
		return nil
	}
	return r.quest.Map(depth - 1)
}

// lastFloor reports whether the hero is on a quest's last map, whose
// stairs end the quest.
func (r *run) lastFloor() bool { return r.quest != nil && r.depth >= len(r.quest.Maps) }

// remember writes what the player has learned to disk, if it can.
func (r *run) remember() {
	if r.prof != nil {
		r.prof.SaveMemory()
	}
}

// floorSeed is the map seed for a depth. The same floor comes back when a
// game is loaded; falling makes new ones.
func (r *run) floorSeed(depth int) uint64 {
	return r.seed*31 + uint64(depth)*7919 + uint64(r.regen)*104729
}

// enter goes to a checkpoint: the hero, the floor and where they stand.
func (r *run) enter(cp checkpoint) (*dungeon.Level, dungeon.Point, dungeon.Dir, error) {
	if cp.Depth < 1 {
		return nil, dungeon.Point{}, 0, fmt.Errorf("no floor %d", cp.Depth)
	}
	r.depth, r.regen, r.hero = cp.Depth, cp.Regen, cp.Hero.Clone()
	l := r.floor(r.depth)
	if cp.Floor == nil {
		return l, l.Start, l.StartDir, nil
	}
	if err := l.Restore(*cp.Floor); err != nil {
		return nil, dungeon.Point{}, 0, err
	}
	if !l.At(cp.At).Walkable() || l.Blocked(cp.At) {
		return nil, dungeon.Point{}, 0, fmt.Errorf("the hero is inside a wall")
	}
	return l, cp.At, cp.Facing & 3, nil
}

// here is a checkpoint of the adventure as it is now.
func (r *run) here(l *dungeon.Level, at dungeon.Point, facing dungeon.Dir) checkpoint {
	s := l.State()
	return checkpoint{Depth: r.depth, Regen: r.regen, Hero: r.hero.Clone(), Floor: &s, At: at, Facing: facing}
}

const maxLog = 50

func (r *run) say(text string, col color.RGBA) {
	r.log = append(r.log, logLine{text, col})
	if len(r.log) > maxLog {
		r.log = r.log[len(r.log)-maxLog:]
	}
}

func (r *run) info(text string) { r.say(text, pal.Ice) }

// logWidth is how many characters fit on a line of the message log.
const logWidth = 74

// sayLong says text over as many log lines as it needs.
func (r *run) sayLong(text string, col color.RGBA) {
	line := ""
	for _, word := range strings.Fields(text) {
		if line != "" && utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) > logWidth {
			r.say(line, col)
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	if line != "" {
		r.say(line, col)
	}
}
