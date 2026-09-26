package words

import (
	"sort"
	"strings"
)

// This file is what the game's Grimoire shows of a set of words: the
// order it lists them in, the rounded percentages and the mistake to
// watch out for. halpwords-server's learner pages use it to show a
// grown-up the numbers the child sees in the game.

// GrimoireSort is an order the Grimoire lists words in.
type GrimoireSort int

// The Grimoire's sorts, in the order the Tab key cycles through them.
const (
	// GrimoireWeakest lists the words Memory.Weakest picks first, weakest
	// first, then the rest from the highest box down: new words last.
	GrimoireWeakest GrimoireSort = iota
	// GrimoireListOrder keeps the words in list order.
	GrimoireListOrder
	// GrimoireAZ sorts the words by prompt, ignoring case.
	GrimoireAZ
	// NumGrimoireSorts is the number of sorts.
	NumGrimoireSorts
)

var grimoireSortNames = [NumGrimoireSorts]string{"weakest first", "list order", "A to Z"}

var grimoireSortIDs = [NumGrimoireSorts]string{"weakest", "list", "az"}

// String is the sort's name as the game shows it ("weakest first").
func (s GrimoireSort) String() string {
	if s < 0 || s >= NumGrimoireSorts {
		return grimoireSortNames[GrimoireWeakest]
	}
	return grimoireSortNames[s]
}

// ID is a short name for the sort, for URLs and forms: "weakest",
// "list" or "az".
func (s GrimoireSort) ID() string {
	if s < 0 || s >= NumGrimoireSorts {
		return grimoireSortIDs[GrimoireWeakest]
	}
	return grimoireSortIDs[s]
}

// ParseGrimoireSort returns the sort whose ID is id.
func ParseGrimoireSort(id string) (GrimoireSort, bool) {
	for s, n := range grimoireSortIDs {
		if n == id {
			return GrimoireSort(s), true
		}
	}
	return GrimoireWeakest, false
}

// Percent rounds a share from 0 to 1 to a whole percentage, as the
// Grimoire shows it (0.125 is 13).
func Percent(share float64) int { return int(share*100 + 0.5) }

// WatchOut is the kind of mistake made most often with the word (the
// first kind, on a tie), which the Grimoire tells the player to watch
// out for; NoMistake if there were none. The Grimoire only shows it for
// a word that is not mastered.
func (c *Card) WatchOut() Mistake {
	if c == nil {
		return NoMistake
	}
	worst, n := NoMistake, 0
	for m, k := range c.Mistakes {
		if k > n {
			worst, n = Mistake(m), k
		}
	}
	return worst
}

// Distinct returns entries with each word (by Key) once, keeping the
// first.
func Distinct(entries []Entry) []Entry {
	var out []Entry
	seen := map[string]bool{}
	for _, e := range entries {
		if k := Key(e); !seen[k] {
			seen[k] = true
			out = append(out, e)
		}
	}
	return out
}

// SortGrimoire returns the order the Grimoire lists entries in, as
// indexes into entries. entries should already be Distinct.
func (m *Memory) SortGrimoire(entries []Entry, by GrimoireSort) []int {
	order := make([]int, len(entries))
	for i := range order {
		order[i] = i
	}
	switch by {
	case GrimoireWeakest:
		rank := map[int]int{}
		for r, i := range m.Weakest(entries, len(entries)) {
			rank[i] = r + 1
		}
		// Weak words first, then the rest by box: new words last.
		sort.SliceStable(order, func(a, b int) bool {
			ia, ib := order[a], order[b]
			ra, rb := rank[ia], rank[ib]
			switch {
			case ra > 0 && rb > 0:
				return ra < rb
			case ra > 0 || rb > 0:
				return ra > 0
			}
			return m.Box(entries[ia]) > m.Box(entries[ib])
		})
	case GrimoireAZ:
		sort.SliceStable(order, func(a, b int) bool {
			return strings.ToLower(entries[order[a]].Prompt) < strings.ToLower(entries[order[b]].Prompt)
		})
	}
	return order
}

// Grimoire is what the game's Grimoire shows for a set of words.
type Grimoire struct {
	Summary Summary
	Sort    GrimoireSort
	// Rows are the words, each once, in the Grimoire's order.
	Rows []GrimoireRow
}

// Right is the share of right answers as a percentage, shown when
// Summary.Seen is not 0.
func (g *Grimoire) Right() int { return Percent(g.Summary.Accuracy()) }

// GrimoireRow is one word in the Grimoire.
type GrimoireRow struct {
	Entry Entry
	// Card is nil for a word not met yet.
	Card *Card
	// Box is the word's Leitner box, 0 if it is new.
	Box int
	// Right is the percentage of right answers (0 when Card is nil).
	Right int
	// Watch is Card.WatchOut().
	Watch Mistake
}

// Met reports whether the word has been answered.
func (r GrimoireRow) Met() bool { return r.Card != nil }

// Mastered reports whether the word is in the top box.
func (r GrimoireRow) Mastered() bool { return r.Box == Boxes }

// Timed reports whether the word has a time to show (Card.AvgSecs).
func (r GrimoireRow) Timed() bool { return r.Card != nil && r.Card.Timed > 0 }

// NewGrimoire lays out entries as the game's Grimoire does: each word
// once (by Key), in the order by.
func (m *Memory) NewGrimoire(entries []Entry, by GrimoireSort) *Grimoire {
	es := Distinct(entries)
	g := &Grimoire{Summary: m.Summarize(es), Sort: by, Rows: make([]GrimoireRow, 0, len(es))}
	for _, i := range m.SortGrimoire(es, by) {
		row := GrimoireRow{Entry: es[i], Card: m.Card(es[i])}
		if c := row.Card; c != nil {
			row.Box = c.Box
			row.Right = Percent(c.Accuracy())
			row.Watch = c.WatchOut()
		}
		g.Rows = append(g.Rows, row)
	}
	return g
}
