package puzzle

import (
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/halpworld/halpwords/pkg/words"
)

// pairs shows PairCount foreign words, each set to one of their English
// meanings in shuffled order. The hero swaps meanings between rows until
// every word has its own.
type pairs struct {
	entries  []words.Entry // one per row, in the order shown
	meanings []string      // the options, in the order they are listed
	start    []int         // the meaning on each row at first
}

// PairCount is how many words a pair-matching puzzle shows.
const PairCount = 4

// newPairs returns nil if the deck does not have PairCount words that
// cannot be mistaken for each other.
func newPairs(deck *words.Deck, rng *rand.Rand) *pairs {
	first, _ := deck.Next()
	all := deck.Entries()
	picked := []words.Entry{first}
	for _, i := range rng.Perm(len(all)) {
		if len(picked) == PairCount {
			break
		}
		if e := all[i]; !clashes(e, picked) {
			picked = append(picked, e)
		}
	}
	if len(picked) < PairCount {
		return nil
	}
	rng.Shuffle(len(picked), func(i, j int) { picked[i], picked[j] = picked[j], picked[i] })
	p := &pairs{entries: picked}
	for _, e := range picked {
		p.meanings = append(p.meanings, e.Prompt)
	}
	// Row i starts with meaning start[i]. Shift by one after sorting a
	// shuffle, so no row starts right.
	order := rng.Perm(PairCount)
	p.start = make([]int, PairCount)
	for k := range order {
		p.start[order[k]] = order[(k+1)%PairCount]
	}
	return p
}

// clashes reports whether e shares an English meaning or a foreign word
// with any of the picked entries, which would make two answers right.
func clashes(e words.Entry, picked []words.Entry) bool {
	for _, p := range picked {
		if strings.EqualFold(e.Prompt, p.Prompt) || e.Answers[0] == p.Answers[0] {
			return true
		}
		for _, a := range e.Answers {
			if has(p.Answers, a) {
				return true
			}
		}
	}
	return false
}

func (p *pairs) Kind() Kind     { return Pairs }
func (p *pairs) Answer() Answer { return Match }
func (p *pairs) Clue() string   { return "" }
func (p *pairs) Word() int      { return -1 }

func (p *pairs) Ask() string { return "Match each word to its meaning:" }

func (p *pairs) Tiles() []string {
	t := make([]string, len(p.entries))
	for i, e := range p.entries {
		t[i] = e.Answers[0]
	}
	return t
}

func (p *pairs) Options() [][]string {
	o := make([][]string, len(p.entries))
	for i := range o {
		o[i] = p.meanings
	}
	return o
}

func (p *pairs) Start() []int { return append([]int(nil), p.start...) }

// Check passes only when every row has its own meaning.
func (p *pairs) Check(a Attempt) Result {
	right := 0
	for i := range p.entries {
		if i < len(a.Choice) && a.Choice[i] == i {
			right++
		}
	}
	r := Result{Result: words.Result{Tier: words.Miss}}
	if right == len(p.entries) {
		r.Tier = words.Perfect
	}
	var sol []string
	for _, e := range p.entries {
		sol = append(sol, e.Answers[0]+" = "+e.Prompt)
	}
	r.Expected = strings.Join(sol, ", ")
	half := (len(sol) + 1) / 2
	r.Solution = []string{strings.Join(sol[:half], ", "), strings.Join(sol[half:], ", ")}
	if r.Tier == words.Miss {
		r.Solution = append(r.Solution, fmt.Sprintf("You matched %d of %d.", right, len(p.entries)))
	}
	return r
}
