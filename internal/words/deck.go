package words

import (
	"math/rand/v2"
	"unicode"
)

// Deck deals words for battles and puzzles. It avoids repeating recent
// words, and brings back words the player got wrong until they get them
// right, so practice goes where it is needed.
type Deck struct {
	entries []Entry
	rng     *rand.Rand
	recent  []int
	review  []int // entries answered wrongly, oldest first
}

// recentLen is how many words must pass before one can be dealt again.
const recentLen = 6

// NewDeck returns a deck over entries, which must not be empty.
func NewDeck(entries []Entry, rng *rand.Rand) *Deck {
	return &Deck{entries: entries, rng: rng}
}

// Len returns the number of words in the deck.
func (d *Deck) Len() int { return len(d.entries) }

// Entries returns every word in the deck, in list order. Entry i is the one
// Next identifies as i. The slice must not be changed.
func (d *Deck) Entries() []Entry { return d.entries }

// Review returns how many words are waiting to be practised again.
func (d *Deck) Review() int { return len(d.review) }

// Next deals a word. The result identifies it for Mark.
func (d *Deck) Next() (Entry, int) {
	i := -1
	// Two times in five, retry a word that was missed, if one has not been
	// seen for a little while.
	if len(d.review) > 0 && d.rng.IntN(5) < 2 {
		for _, r := range d.review {
			if !d.isRecent(r, recentLen/2) {
				i = r
				break
			}
		}
	}
	if i < 0 {
		i = d.rng.IntN(len(d.entries))
		for try := 0; try < 10 && d.isRecent(i, min(recentLen, len(d.entries)-1)); try++ {
			i = d.rng.IntN(len(d.entries))
		}
	}
	d.recent = append(d.recent, i)
	if len(d.recent) > recentLen {
		d.recent = d.recent[1:]
	}
	return d.entries[i], i
}

func (d *Deck) isRecent(i, n int) bool {
	for k := len(d.recent) - 1; k >= 0 && k >= len(d.recent)-n; k-- {
		if d.recent[k] == i {
			return true
		}
	}
	return false
}

// Mark records whether word i (from Next) was answered well enough.
func (d *Deck) Mark(i int, ok bool) {
	for k, r := range d.review {
		if r == i {
			if ok {
				d.review = append(d.review[:k], d.review[k+1:]...)
			}
			return
		}
	}
	if !ok {
		d.review = append(d.review, i)
	}
}

// Blank hides about share (0 to 1) of the letters of s behind underscores
// for a fill-in puzzle. The first letter of each word and all punctuation
// stay visible, and at least one letter is always hidden when s has any to
// hide.
func Blank(s string, share float64, rng *rand.Rand) string {
	rs := []rune(s)
	var can []int
	start := true
	for i, r := range rs {
		letter := unicode.IsLetter(r)
		if letter && !start {
			can = append(can, i)
		}
		start = !letter
	}
	if len(can) == 0 {
		return s
	}
	rng.Shuffle(len(can), func(a, b int) { can[a], can[b] = can[b], can[a] })
	n := min(len(can), max(1, int(float64(len(can))*share+0.5)))
	for _, i := range can[:n] {
		rs[i] = '_'
	}
	return string(rs)
}
