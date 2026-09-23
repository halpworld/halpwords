// Package puzzle makes the word puzzles that lock sealed doors and chests.
// Unlike battles, puzzles have no clock: they are for careful spelling and
// for looking at words in new ways. It has no Ebitengine dependency.
package puzzle

import (
	"math/rand/v2"

	"github.com/halpworld/halpwords/internal/words"
)

// Kind is a type of puzzle.
type Kind int

const (
	Reverse   Kind = iota // read a foreign word, type its meaning in English
	OddOneOut             // pick the word that is not in the group
	Anagram               // unscramble the letters of a word
	Missing               // fill in the hidden letters of a word
	Spell                 // spell a word from its meaning, with no help
	Pairs                 // match foreign words to their meanings
	Riddle                // solve an English riddle in the foreign language
	Tumbler               // turn letter wheels to spell a word
	Crossword             // fill in words that cross on a shared letter
)

func (k Kind) String() string {
	return [...]string{"reverse rune", "odd one out", "anagram", "missing letters", "spelling",
		"pair matching", "riddle", "tumbler lock", "crossword"}[k]
}

// Lock is what a puzzle opens.
type Lock int

const (
	Door  Lock = iota // a rune-sealed door
	Chest             // a locked chest
)

// Answer is how the hero answers a puzzle.
type Answer int

const (
	Foreign Answer = iota // type a word in the language being learned
	Native                // type a word in English
	Pick                  // choose one of the tiles
	Match                 // give each tile one of the options (a Chooser)
	Dial                  // turn each wheel to one of its options (a Chooser)
	Grid                  // type the words of a crossword (a Crossworder)
)

// Attempt is the hero's answer.
type Attempt struct {
	Text          string   // for Foreign and Native puzzles
	Pick          int      // the chosen tile, for Pick puzzles
	Choice        []int    // the option set on each slot, for Match and Dial
	Texts         []string // each word typed, for Grid puzzles
	UsedBackspace bool
}

// Result is how an attempt went.
type Result struct {
	words.Result
	// Solution explains the answer, one line each.
	Solution []string
}

// Passed reports whether the lock opens. Accent slips are forgiven; the
// solution shows the marks.
func (r Result) Passed() bool { return r.Tier >= words.AccentSlip }

// Puzzle is one word puzzle.
type Puzzle interface {
	Kind() Kind
	Answer() Answer
	// Ask is the instruction, such as "Spell in French:".
	Ask() string
	// Clue is shown large under the instruction. It may be empty.
	Clue() string
	// Tiles are letters to unscramble (Anagram), words to pick from
	// (OddOneOut) or words to match (Pairs). Other puzzles have none.
	Tiles() []string
	Check(Attempt) Result
	// Word is the deck index of the word being practised, for Deck.Mark, or
	// -1 when the puzzle is not about one word.
	Word() int
}

// Chooser is a puzzle answered by setting each of its slots to one of
// that slot's options: the meaning of each word in Pairs, or the letter on
// each wheel in Tumbler.
type Chooser interface {
	Puzzle
	// Options lists each slot's options. A slot with one option is fixed.
	Options() [][]string
	// Start is the option each slot is set to when the puzzle opens.
	Start() []int
}

// Kinds returns the puzzles a lock can have on floor depth. Doors test
// knowing words; chests test spelling them, and hold the better rewards.
func Kinds(lock Lock, depth int) []Kind {
	if lock == Door {
		k := []Kind{Reverse, Anagram, OddOneOut, Pairs, Riddle}
		if depth >= 2 {
			k = append(k, Spell)
		}
		return k
	}
	k := []Kind{Missing, Anagram, Tumbler}
	if depth >= CrosswordDepth {
		k = append(k, Spell, Crossword)
	}
	return k
}

// New makes a random puzzle for a lock on floor depth, with words dealt from
// deck.
func New(lock Lock, depth int, deck *words.Deck, lang *words.Language, rng *rand.Rand) Puzzle {
	ks := Kinds(lock, depth)
	return Make(ks[rng.IntN(len(ks))], lock, depth, deck, lang, rng)
}

// Make makes a puzzle of kind k. When the word lists cannot make that kind
// (a word too short to scramble, or lists without groups), it makes a
// spelling puzzle instead.
func Make(k Kind, lock Lock, depth int, deck *words.Deck, lang *words.Language, rng *rand.Rand) Puzzle {
	var p Puzzle
	switch k {
	case OddOneOut:
		if o := newOddOneOut(deck.Entries(), rng); o != nil {
			p = o
		}
	case Pairs:
		if m := newPairs(deck, rng); m != nil {
			p = m
		}
	case Riddle:
		if t := newRiddle(deck, lang, rng); t != nil {
			p = t
		}
	case Tumbler:
		if t := newTumbler(deck, depth, lang, rng); t != nil {
			p = t
		}
	case Crossword:
		if c := newCrossword(deck, depth, lang, rng); c != nil {
			p = c
		}
	}
	if p != nil {
		return p
	}
	e, id := deck.Next()
	var t *typed // not a Puzzle: a nil *typed in an interface is not nil
	switch k {
	case Reverse:
		t = newReverse(e, id, deck.Entries())
	case Anagram:
		t = newAnagram(e, id, lock, depth, deck.Entries(), lang, rng)
	case Missing:
		t = newMissing(e, id, depth, lang, rng)
	}
	if t == nil {
		t = newSpell(e, id, lang)
	}
	return t
}
