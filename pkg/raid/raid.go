// Package raid holds the rules of a Boss Raid (halpwords-server's play
// rooms, its docs/api/play.md): a whole class fights one boss together.
// The server deals each raider their own words, grades every answer with
// Grade (the game's own grading, words.Grade), and takes Damage off the
// boss's health; now and then the boss attacks and every raider types a
// dodge in DodgeTime. The class wins when the boss's health reaches
// nothing before TimeLimit.
//
// The server is the only judge: a raiding game sends what was typed and
// shows what the server says, so the numbers here are the ones both
// sides use. Damage and dodges follow the game's battle formulas
// (internal/combat) without the speed bonus, since a network delay would
// make speed unfair.
//
// Like the other pkg/ packages it has no Ebitengine dependency.
package raid

import (
	"math"
	"time"

	"github.com/halpworld/halpwords/internal/combat"
	"github.com/halpworld/halpwords/internal/dungeon"
	"github.com/halpworld/halpwords/pkg/proc"
	"github.com/halpworld/halpwords/pkg/words"
)

// TimeLimit is how long a raid lasts at most. When it is up and the boss
// still stands, the boss escapes.
const TimeLimit = 10 * time.Minute

// MinRaiders and MaxRaiders are how many raiders a raid needs to start
// and may have: a class, from a pair to a large one.
const (
	MinRaiders = 2
	MaxRaiders = 40
)

// MaxAnswer is the longest answer a raider may send, in characters: the
// game's typing field holds no more.
const MaxAnswer = 40

// HPPerRaider is the boss's health for each raider. A raider answering
// steadily deals about 100 a minute, so a class that keeps typing
// defeats the boss in about five minutes.
const HPPerRaider = 500

// BossHP is the boss's health for a raid of raiders.
func BossHP(raiders int) int { return HPPerRaider * max(raiders, 1) }

// Hit is the damage of a perfect answer with no streak.
const Hit = 10

// Damage is what an answer graded tier deals to the boss, for a raider
// on a streak of good answers: the battle's accuracy and combo, with no
// speed bonus.
func Damage(tier words.Tier, streak int) int {
	return int(math.Round(Hit * combat.Accuracy(tier) * combat.Combo(streak)))
}

// Streak is a raider's streak after an answer graded tier, as in the
// game's battles: a correct answer builds it, a graze or a miss breaks
// it, and an accent slip does neither.
func Streak(streak int, tier words.Tier) int {
	switch {
	case tier >= words.Correct:
		return streak + 1
	case tier < words.AccentSlip:
		return 0
	}
	return streak
}

// FirstAttack is when the boss first attacks, after the raid starts.
const FirstAttack = 30 * time.Second

// AttackEvery is the time between the boss's attacks, which grow more
// frequent as it weakens (combat.BossPhase).
func AttackEvery(hp, maxHP int) time.Duration {
	switch combat.BossPhase(hp, maxHP) {
	case 2:
		return 20 * time.Second
	case 1:
		return 30 * time.Second
	}
	return 40 * time.Second
}

// Slack is the time a dodge is given on top of the battle's, for the
// network both ways.
const Slack = 2 * time.Second

// DodgeTime is how long a raider has to type a dodge whose answer is n
// characters long: the battle's time to defend, and Slack.
func DodgeTime(n int) time.Duration {
	return time.Duration(combat.DefendTime(n)*float64(time.Second)) + Slack
}

// Dodged reports whether a dodge graded tier gets out of the way. A
// graze or a miss doesn't, and neither does a dodge not typed in time.
func Dodged(tier words.Tier) bool { return tier >= words.AccentSlip }

// StunTime is how long a raider who fails to dodge waits before their
// next word.
const StunTime = 5 * time.Second

// Grade grades an answer typed to entry in the language with code
// langCode, with the language's default rules: the game's own grading.
// A language the game doesn't know grades strictly, with no articles to
// leave out.
func Grade(typed string, entry words.Entry, langCode string, usedBackspace bool) words.Result {
	lang, ok := words.Lookup(langCode)
	if !ok {
		lang = &words.Language{Code: langCode}
	}
	return words.Grade(typed, entry, lang, lang.Defaults, usedBackspace)
}

// Boss is who a raid fights: one of the dungeon's bosses.
type Boss struct {
	Name   string
	family dungeon.Family
	hue    int
	seed   uint64
}

// BossFor picks the boss for a raid from its seed.
func BossFor(seed uint64) Boss {
	k := dungeon.BossKinds[seed%uint64(len(dungeon.BossKinds))]
	return Boss{Name: k.Name, family: k.Family, hue: k.Hue, seed: seed}
}

// Frames is how many frames the boss's sprite has.
const Frames = 2

// Sprite draws the boss's sprite for frame (0 or 1), crowned as in the
// dungeon.
func (b Boss) Sprite(frame int) *proc.Indexed {
	return proc.Crown(proc.MonsterSprite(b.family, b.hue, b.seed, frame%Frames))
}
