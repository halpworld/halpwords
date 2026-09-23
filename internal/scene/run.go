package scene

import (
	"image/color"
	"math/rand/v2"
	"os"
	"strconv"
	"time"

	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/pal"
	"github.com/halpworld/halpwords/internal/proc"
	"github.com/halpworld/halpwords/internal/words"
)

// hero is the player character's stats.
type hero struct {
	HP, MaxHP int
	ATK       int
	Level, XP int
	Gold      int
	Potions   int
	Streak    int // good answers in a row, for the combo bonus
}

func newHero() hero {
	return hero{HP: 30, MaxHP: 30, ATK: 6, Level: 1, Potions: 1}
}

// NextXP is the experience needed for the next level.
func (h *hero) NextXP() int { return 12 * h.Level }

// GainXP adds experience and returns the number of levels gained. Each
// level raises HP and attack and heals fully.
func (h *hero) GainXP(xp int) int {
	h.XP += xp
	n := 0
	for h.XP >= h.NextXP() {
		h.XP -= h.NextXP()
		h.Level++
		h.MaxHP += 6
		h.ATK += 2
		h.HP = h.MaxHP
		n++
	}
	return n
}

// Heal restores HP, up to the maximum.
func (h *hero) Heal(n int) { h.HP = min(h.MaxHP, h.HP+n) }

// PotionHeal is how much a potion restores.
func (h *hero) PotionHeal() int { return max(12, h.MaxHP/2) }

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
	// saved is the checkpoint: the hero as they arrived on the current
	// floor. Dying returns here.
	saved hero
	depth int
	log   []logLine
	greek bool // Greek keys on, for Greek

	// seenTraits are the monster traits the hero has been told about.
	seenTraits dungeon.Trait
	sound      *game.Sound
}

func newRun(ctx *game.Context, lang *words.Language) *run {
	seed := uint64(time.Now().UnixNano())
	// HALPWORDS_SEED replays a dungeon, for testing and bug reports.
	if v, err := strconv.ParseUint(os.Getenv("HALPWORDS_SEED"), 10, 64); err == nil {
		seed = v
	}
	return startRun(ctx, lang, seed)
}

// startRun begins a run in lang through the dungeon made from seed.
func startRun(ctx *game.Context, lang *words.Language, seed uint64) *run {
	var entries []words.Entry
	for _, l := range ctx.ListsFor(lang.Code) {
		entries = append(entries, l.Entries...)
	}
	src := proc.NewPCG(seed)
	rng := rand.New(src)
	r := &run{
		lang:  lang,
		deck:  words.NewDeck(entries, rng),
		rng:   rng,
		src:   src,
		seed:  seed,
		hero:  newHero(),
		depth: 1,
		greek: lang.Script == words.ScriptGreek,
		sound: ctx.Sound,
	}
	r.saved = r.hero
	return r
}

// floorSeed is the map seed for a depth, so a floor is the same when the
// hero comes back to it after dying.
func (r *run) floorSeed(depth int) uint64 { return r.seed*31 + uint64(depth)*7919 }

const maxLog = 50

func (r *run) say(text string, col color.RGBA) {
	r.log = append(r.log, logLine{text, col})
	if len(r.log) > maxLog {
		r.log = r.log[len(r.log)-maxLog:]
	}
}

func (r *run) info(text string) { r.say(text, pal.Ice) }
