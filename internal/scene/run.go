package scene

import (
	"fmt"
	"image/color"
	"math/rand/v2"
	"os"
	"strconv"
	"time"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/proc"
	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/internal/words"
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
}

func newRun(ctx *game.Context, lang *words.Language, class rpg.Class) *run {
	seed := uint64(time.Now().UnixNano())
	// HALPWORDS_SEED replays a dungeon, for testing and bug reports.
	if v, err := strconv.ParseUint(os.Getenv("HALPWORDS_SEED"), 10, 64); err == nil {
		seed = v
	}
	return startRun(ctx, lang, class, seed)
}

// startRun begins a run in lang as a hero of class through the dungeon made
// from seed.
func startRun(ctx *game.Context, lang *words.Language, class rpg.Class, seed uint64) *run {
	var entries []words.Entry
	for _, l := range ctx.ListsFor(lang.Code) {
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
	}
	r.shrine = checkpoint{Depth: 1, Hero: r.hero.Clone()}
	return r
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
	l := dungeon.Generate(r.floorSeed(r.depth), r.depth)
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
