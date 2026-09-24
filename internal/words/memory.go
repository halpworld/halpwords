package words

import (
	"sort"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// Boxes is the number of Leitner boxes. A word the player has never seen is
// in box 0; the first answer puts it in box 1, perfect answers move it up a
// box and misses send it back to box 1. A word in the top box is mastered.
const Boxes = 5

// boxGap is how many answers (in the same language) must pass before a word
// in each box is due again. Box 0 words are new, not due.
var boxGap = [Boxes + 1]int{0, 3, 8, 20, 50, 120}

// Card is what the player knows of one word.
type Card struct {
	Box int // 0 (new) to Boxes
	Due int // the Memory's Clock at which it is due again

	Seen    int // answers given
	Right   int // answers that were Correct or Perfect
	Perfect int
	Misses  int
	// Timed answers are the ones given against a clock, in battles; Secs
	// adds up the time they took.
	Timed int
	Secs  float64
	// Mistakes counts the kinds of wrong answers.
	Mistakes [NumMistakes]int `json:",omitempty"`
}

// Accuracy is the share of answers that were right, from 0 to 1.
func (c *Card) Accuracy() float64 {
	if c.Seen == 0 {
		return 0
	}
	return float64(c.Right) / float64(c.Seen)
}

// AvgSecs is the average time a timed answer took, or 0 if there were none.
func (c *Card) AvgSecs() float64 {
	if c.Timed == 0 {
		return 0
	}
	return c.Secs / float64(c.Timed)
}

// Memory is what the player knows of the words in one language: a Leitner
// box and statistics for each word. It lasts across adventures.
type Memory struct {
	// Clock counts the answers given in this language. Words fall due by
	// it, so they come back after so many other words, however long the
	// player is away.
	Clock int
	Cards map[string]*Card
}

// NewMemory returns an empty memory.
func NewMemory() *Memory { return &Memory{Cards: map[string]*Card{}} }

// Key identifies a word across word list changes: its prompt and first
// answer.
func Key(e Entry) string {
	a := ""
	if len(e.Answers) > 0 {
		a = e.Answers[0]
	}
	return norm.NFC.String(strings.TrimSpace(e.Prompt) + " = " + strings.TrimSpace(a))
}

// Card returns the card for e, or nil if the word has never been answered.
func (m *Memory) Card(e Entry) *Card {
	if m == nil {
		return nil
	}
	return m.Cards[Key(e)]
}

// Box returns e's box: 0 when it has never been answered.
func (m *Memory) Box(e Entry) int {
	if c := m.Card(e); c != nil {
		return c.Box
	}
	return 0
}

// IsDue reports whether e has been answered before and is due to be
// practised again.
func (m *Memory) IsDue(e Entry) bool {
	c := m.Card(e)
	return c != nil && c.Box > 0 && c.Due <= m.Clock
}

// Answer is one answer to a word, for Memory.Record.
type Answer struct {
	Tier   Tier
	Hinted bool    // a hint showed some of the letters
	Secs   float64 // how long it took, when Timed
	Timed  bool
	// Mistake is the kind of mistake, for answers that were not right.
	Mistake Mistake
}

// Record moves e between boxes after an answer and updates its statistics.
// A perfect answer moves it up a box; a correct one (fixed with backspace)
// keeps it where it is; a slip or graze moves it down one box, and a miss
// sends it back to box 1. A hinted answer never moves a word up.
func (m *Memory) Record(e Entry, a Answer) {
	k := Key(e)
	c := m.Cards[k]
	if c == nil {
		c = &Card{}
		if m.Cards == nil {
			m.Cards = map[string]*Card{}
		}
		m.Cards[k] = c
	}
	m.Clock++
	c.Seen++
	switch {
	case a.Tier == Perfect && !a.Hinted:
		c.Box++
	case a.Tier >= Correct:
		// stays
	case a.Tier == Miss:
		c.Box = 1
	default:
		c.Box--
	}
	if a.Hinted && c.Box > 1 {
		c.Box--
	}
	c.Box = max(1, min(Boxes, c.Box))
	c.Due = m.Clock + boxGap[c.Box]
	if a.Tier >= Correct {
		c.Right++
	}
	if a.Tier == Perfect && !a.Hinted {
		c.Perfect++
	}
	if a.Tier == Miss {
		c.Misses++
	}
	if a.Tier < Correct && a.Mistake > NoMistake && a.Mistake < NumMistakes {
		c.Mistakes[a.Mistake]++
	}
	if a.Timed && a.Secs > 0 {
		c.Timed++
		c.Secs += a.Secs
	}
}

// Summary is what the player knows of a set of words.
type Summary struct {
	Words   int            // words in the set
	InBox   [Boxes + 1]int // words in each box; box 0 is new words
	Seen    int            // answers given
	Right   int            // answers that were right
	Timed   int
	Secs    float64
	Mistake [NumMistakes]int
}

// Mastered is the number of words in the top box.
func (s Summary) Mastered() int { return s.InBox[Boxes] }

// Accuracy is the share of answers that were right, from 0 to 1.
func (s Summary) Accuracy() float64 {
	if s.Seen == 0 {
		return 0
	}
	return float64(s.Right) / float64(s.Seen)
}

// AvgSecs is the average time of a timed answer, or 0.
func (s Summary) AvgSecs() float64 {
	if s.Timed == 0 {
		return 0
	}
	return s.Secs / float64(s.Timed)
}

// CommonMistake is the kind of mistake made most often, or NoMistake.
func (s Summary) CommonMistake() Mistake {
	best := NoMistake
	for k := NoMistake + 1; k < NumMistakes; k++ {
		if s.Mistake[k] > 0 && (best == NoMistake || s.Mistake[k] > s.Mistake[best]) {
			best = k
		}
	}
	return best
}

// Summarize describes what the player knows of entries.
func (m *Memory) Summarize(entries []Entry) Summary {
	s := Summary{Words: len(entries)}
	seen := map[string]bool{}
	for _, e := range entries {
		k := Key(e)
		if seen[k] {
			s.Words--
			continue
		}
		seen[k] = true
		c := m.Card(e)
		if c == nil {
			s.InBox[0]++
			continue
		}
		s.InBox[c.Box]++
		s.Seen += c.Seen
		s.Right += c.Right
		s.Timed += c.Timed
		s.Secs += c.Secs
		for i, n := range c.Mistakes {
			s.Mistake[i] += n
		}
	}
	return s
}

// Weakest orders words by how much practice they need: answered words in
// low boxes with many misses come first. Words never answered are left out.
// It returns indexes into entries, at most n of them.
func (m *Memory) Weakest(entries []Entry, n int) []int {
	type scored struct {
		i     int
		score float64
	}
	var out []scored
	seen := map[string]bool{}
	for i, e := range entries {
		k := Key(e)
		c := m.Card(e)
		if c == nil || seen[k] {
			continue
		}
		seen[k] = true
		// Low boxes first; then the share of misses; then how often.
		sc := float64(Boxes-c.Box)*10 + (1-c.Accuracy())*8 + float64(min(c.Misses, 10))*0.2
		if c.Box == Boxes && c.Misses == 0 {
			continue // mastered and never missed
		}
		out = append(out, scored{i, sc})
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].score > out[b].score })
	ids := make([]int, 0, min(n, len(out)))
	for _, s := range out[:min(n, len(out))] {
		ids = append(ids, s.i)
	}
	return ids
}
