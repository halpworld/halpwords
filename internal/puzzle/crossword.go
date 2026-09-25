package puzzle

import (
	"math/rand/v2"
	"strings"

	"github.com/halpworld/halpwords/internal/words"
)

// Crossword depths: chests have crosswords of two words from
// CrosswordDepth, and of three from Crossword3Depth.
const (
	CrosswordDepth  = 3
	Crossword3Depth = 6
)

// Crossword word lengths, in letters.
const (
	MinCross = 3
	MaxCross = 8
)

// Placed is one word of a crossword.
type Placed struct {
	X, Y int // the first letter's cell
	Down bool
	Len  int    // in letters
	Clue string // the English word
}

// Cell returns the cell of letter k.
func (p Placed) Cell(k int) (x, y int) {
	if p.Down {
		return p.X, p.Y + k
	}
	return p.X + k, p.Y
}

// Crossworder is a Grid puzzle: words typed into a grid, crossing on
// shared letters. Attempt.Texts holds one text per word, in order.
type Crossworder interface {
	Puzzle
	// Layout returns the grid size and the words on it.
	Layout() (w, h int, placed []Placed)
}

// crossword is an across word crossed by one or two down words. The down
// words are at least two columns apart, so they never touch.
type crossword struct {
	w, h    int
	placed  []Placed
	entries []words.Entry
	answers []string // the words without articles
	id      int
	lang    *words.Language
	rules   words.Rules
}

// newCrossword returns nil if the deck has no words that cross.
func newCrossword(deck *words.Deck, depth int, lang *words.Language, rng *rand.Rand) *crossword {
	fits := func(e words.Entry) bool {
		n := 0
		for _, u := range units(bare(e, lang)) {
			if !isLetter(u) {
				return false
			}
			n++
		}
		return n >= MinCross && n <= MaxCross
	}
	across, id, ok := deck.NextWhere(fits)
	if !ok {
		return nil
	}
	c := &crossword{id: id, lang: lang, rules: lang.Defaults}
	c.add(across, Placed{Len: len(units(bare(across, lang))), Clue: across.Prompt})
	a := units(c.answers[0])

	// cross finds a down word crossing the across word at a letter at
	// least two columns from the columns in used.
	all := deck.Entries()
	cross := func(used []int) bool {
		for _, n := range rng.Perm(len(all)) {
			e := all[n]
			if !fits(e) || c.clashes(e) {
				continue
			}
			d := units(bare(e, lang))
			type hit struct{ i, j int }
			var hits []hit
			for i := range a {
				far := true
				for _, u := range used {
					far = far && (i-u >= 2 || u-i >= 2)
				}
				for j := range d {
					if far && sameLetter(a[i], d[j]) {
						hits = append(hits, hit{i, j})
					}
				}
			}
			if len(hits) > 0 {
				h := hits[rng.IntN(len(hits))]
				c.add(e, Placed{X: h.i, Y: -h.j, Down: true, Len: len(d), Clue: e.Prompt})
				return true
			}
		}
		return false
	}
	if !cross(nil) {
		return nil
	}
	if depth >= Crossword3Depth {
		cross([]int{c.placed[1].X}) // two words if no third fits
	}

	// Move the grid so its top-left cell is (0, 0).
	top, bottom := 0, 1
	for _, p := range c.placed {
		if p.Down {
			top, bottom = min(top, p.Y), max(bottom, p.Y+p.Len)
		}
	}
	for i := range c.placed {
		c.placed[i].Y -= top
	}
	c.w, c.h = c.placed[0].Len, bottom-top
	return c
}

func (c *crossword) add(e words.Entry, p Placed) {
	c.entries = append(c.entries, e)
	c.answers = append(c.answers, bare(e, c.lang))
	c.placed = append(c.placed, p)
}

// clashes reports whether e is, or means the same as, a word already on
// the grid.
func (c *crossword) clashes(e words.Entry) bool {
	for i, o := range c.entries {
		if strings.EqualFold(e.Prompt, o.Prompt) || strings.EqualFold(bare(e, c.lang), c.answers[i]) {
			return true
		}
	}
	return false
}

// bare returns e's first answer without its article.
func bare(e words.Entry, lang *words.Language) string {
	_, rest := splitArticle(e.Answers[0], lang)
	return rest
}

func (c *crossword) Kind() Kind      { return Crossword }
func (c *crossword) Answer() Answer  { return Grid }
func (c *crossword) Ask() string     { return "Fill in the " + c.lang.Name + " crossword:" }
func (c *crossword) Clue() string    { return "" }
func (c *crossword) Tiles() []string { return nil }

// Word is the across word, the one dealt from the deck.
func (c *crossword) Word() int { return c.id }

func (c *crossword) Layout() (w, h int, placed []Placed) {
	return c.w, c.h, append([]Placed(nil), c.placed...)
}

// Check grades each word; the crossword is as good as its worst word.
func (c *crossword) Check(a Attempt) Result {
	r := Result{Result: words.Result{Tier: words.Perfect}}
	var want []string
	for i, e := range c.entries {
		typed := ""
		if i < len(a.Texts) {
			typed = a.Texts[i]
		}
		res := words.Grade(typed, only(e, c.answers[i]), c.lang, c.rules, a.UsedBackspace)
		r.Tier = min(r.Tier, res.Tier)
		r.MarkError = r.MarkError || res.MarkError
		want = append(want, c.answers[i])
		r.Solution = append(r.Solution, e.Prompt+" = "+c.answers[i])
	}
	r.Expected = strings.Join(want, ", ")
	return r
}
