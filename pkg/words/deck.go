package words

import (
	"math"
	"math/rand/v2"
	"unicode"
)

// Deck deals words for battles and puzzles. It avoids repeating recent
// words, and brings back words the player got wrong until they get them
// right, so practice goes where it is needed. With a Memory, it deals
// mostly words that are due for practice, and some new ones.
type Deck struct {
	entries []Entry
	rng     *rand.Rand
	recent  []int
	review  []int // entries answered wrongly, oldest first
	mem     *Memory
}

// recentLen is how many words must pass before one can be dealt again.
const recentLen = 6

// NewDeck returns a deck over entries, which must not be empty.
func NewDeck(entries []Entry, rng *rand.Rand) *Deck {
	return &Deck{entries: entries, rng: rng}
}

// SetMemory makes the deck deal by what the player knows, and record
// answers in m. A nil m deals at random.
func (d *Deck) SetMemory(m *Memory) { d.mem = m }

// Memory returns the deck's memory, which may be nil.
func (d *Deck) Memory() *Memory { return d.mem }

// Len returns the number of words in the deck.
func (d *Deck) Len() int { return len(d.entries) }

// Entries returns every word in the deck, in list order. Entry i is the one
// Next identifies as i. The slice must not be changed.
func (d *Deck) Entries() []Entry { return d.entries }

// Review returns how many words are waiting to be practised again.
func (d *Deck) Review() int { return len(d.review) }

// Missed returns the words waiting to be practised again, most recently
// missed first. Each is an id as Next returns.
func (d *Deck) Missed() []int {
	out := make([]int, 0, len(d.review))
	for i := len(d.review) - 1; i >= 0; i-- {
		out = append(out, d.review[i])
	}
	return out
}

// Next deals a word. The result identifies it for Mark.
func (d *Deck) Next() (Entry, int) {
	e, i, _ := d.NextWhere(nil)
	return e, i
}

// NextWhere deals a word that ok accepts (any word when ok is nil), the
// same way Next does. It reports false when ok accepts no word.
func (d *Deck) NextWhere(ok func(Entry) bool) (Entry, int, bool) {
	return d.NextNear(-1, ok)
}

// NextNear deals a word that ok accepts, preferring words whose Difficulty
// is near target. A target below 0 has no preference.
func (d *Deck) NextNear(target float64, ok func(Entry) bool) (Entry, int, bool) {
	fits := func(i int) bool { return ok == nil || ok(d.entries[i]) }
	var can []int
	for i := range d.entries {
		if fits(i) {
			can = append(can, i)
		}
	}
	if len(can) == 0 {
		return Entry{}, -1, false
	}
	i := -1
	// Two times in five, retry a word that was missed, if one has not been
	// seen for a little while.
	if len(d.review) > 0 && d.rng.IntN(5) < 2 {
		for _, r := range d.review {
			if fits(r) && !d.isRecent(r, recentLen/2) {
				i = r
				break
			}
		}
	}
	if i < 0 {
		i = d.pick(d.pool(can), can, target)
	}
	d.recent = append(d.recent, i)
	if len(d.recent) > recentLen {
		d.recent = d.recent[1:]
	}
	return d.entries[i], i, true
}

// Shares of deals, in percent, that go to words that are due and to new
// words, when there are some. The rest can be any word.
const (
	dueShare = 60
	newShare = 25
)

// pool chooses the words to deal from: with a memory, mostly words that are
// due, sometimes new ones, and otherwise any word in can.
func (d *Deck) pool(can []int) []int {
	if d.mem == nil {
		return can
	}
	var due, fresh []int
	for _, i := range can {
		e := d.entries[i]
		switch c := d.mem.Card(e); {
		case c == nil:
			fresh = append(fresh, i)
		case c.Box > 0 && c.Due <= d.mem.Clock:
			due = append(due, i)
		}
	}
	r := d.rng.IntN(100)
	switch {
	case len(due) > 0 && r < dueShare:
		return due
	case len(fresh) > 0 && r < dueShare+newShare:
		return fresh
	}
	return can
}

// pick chooses a word from pool that has not been dealt lately, or from can
// if every word in pool has. With a target, it looks at a few words and
// takes the one nearest the target difficulty.
func (d *Deck) pick(pool, can []int, target float64) int {
	gap := min(recentLen, len(can)-1)
	fresh := func(ids []int) []int {
		var out []int
		for _, i := range ids {
			if !d.isRecent(i, gap) {
				out = append(out, i)
			}
		}
		return out
	}
	from := fresh(pool)
	if len(from) == 0 {
		from = fresh(can)
	}
	if len(from) == 0 {
		from = pool
	}
	i := from[d.rng.IntN(len(from))]
	if target < 0 {
		return i
	}
	best := math.Abs(Difficulty(d.entries[i]) - target)
	for k := 0; k < 3; k++ {
		j := from[d.rng.IntN(len(from))]
		if dj := math.Abs(Difficulty(d.entries[j]) - target); dj < best {
			i, best = j, dj
		}
	}
	return i
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

// Answer records an answer to word i (from Next): a word answered wrongly,
// or with a hint, comes back soon, and the memory, if any, learns how it
// went.
func (d *Deck) Answer(i int, a Answer) {
	if i < 0 || i >= len(d.entries) {
		return
	}
	d.Mark(i, a.Tier >= Correct && !a.Hinted)
	if d.mem != nil {
		d.mem.Record(d.entries[i], a)
	}
}

// DeckState is the deck's memory of recent and missed words, for saving.
type DeckState struct {
	Recent []int `json:",omitempty"`
	Review []int `json:",omitempty"`
}

// State returns the deck's memory, to be restored with SetState.
func (d *Deck) State() DeckState {
	return DeckState{Recent: append([]int(nil), d.recent...), Review: append([]int(nil), d.review...)}
}

// SetState restores the deck's memory. Words that are no longer in the deck,
// because a word list got shorter, are dropped.
func (d *Deck) SetState(s DeckState) {
	keep := func(ids []int) []int {
		var out []int
		for _, i := range ids {
			if i >= 0 && i < len(d.entries) {
				out = append(out, i)
			}
		}
		return out
	}
	d.recent, d.review = keep(s.Recent), keep(s.Review)
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
