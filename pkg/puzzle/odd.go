package puzzle

import (
	"math/rand/v2"
	"sort"
	"strings"

	"github.com/halpworld/halpwords/pkg/words"
)

// oddOneOut shows four words: three from one group of a word list (the
// "## tag" lines) and one from another. The hero picks the odd one.
type oddOneOut struct {
	tag     string
	entries []words.Entry // in the order shown
	odd     int
}

// OddGroup is how many words of the same group an odd-one-out puzzle shows.
const OddGroup = 3

// newOddOneOut returns nil if the lists do not have two groups, one of them
// with at least OddGroup words.
func newOddOneOut(all []words.Entry, rng *rand.Rand) *oddOneOut {
	groups := map[string][]words.Entry{}
	for _, e := range all {
		if e.Tag != "" {
			groups[e.Tag] = append(groups[e.Tag], e)
		}
	}
	var tags []string
	for t, es := range groups {
		if len(distinct(es)) >= OddGroup {
			tags = append(tags, t)
		}
	}
	if len(tags) == 0 || len(groups) < 2 {
		return nil
	}
	sort.Strings(tags) // map order is random; keep seeded games repeatable
	tag := tags[rng.IntN(len(tags))]

	group := distinct(groups[tag])
	rng.Shuffle(len(group), func(i, j int) { group[i], group[j] = group[j], group[i] })
	group = group[:OddGroup]

	// The odd word must not also be a word of the group, or it would not
	// be odd: "sword" might be in both "things" and "dungeon".
	inGroup := map[string]bool{}
	for _, e := range groups[tag] {
		inGroup[e.Prompt] = true
		for _, a := range e.Answers {
			inGroup[a] = true
		}
	}
	var others []words.Entry
	for _, e := range all {
		if e.Tag != "" && e.Tag != tag && !inGroup[e.Prompt] && !inGroup[e.Answers[0]] {
			others = append(others, e)
		}
	}
	if len(others) == 0 {
		return nil
	}
	p := &oddOneOut{tag: tag, entries: append(group, others[rng.IntN(len(others))])}
	rng.Shuffle(len(p.entries), func(i, j int) { p.entries[i], p.entries[j] = p.entries[j], p.entries[i] })
	for i, e := range p.entries {
		if e.Tag != tag {
			p.odd = i
		}
	}
	return p
}

// distinct returns the entries whose first answers differ, so no two tiles
// look the same.
func distinct(es []words.Entry) []words.Entry {
	seen := map[string]bool{}
	var out []words.Entry
	for _, e := range es {
		if !seen[e.Answers[0]] {
			seen[e.Answers[0]] = true
			out = append(out, e)
		}
	}
	return out
}

func (p *oddOneOut) Kind() Kind     { return OddOneOut }
func (p *oddOneOut) Answer() Answer { return Pick }
func (p *oddOneOut) Clue() string   { return "" }
func (p *oddOneOut) Word() int      { return -1 }

func (p *oddOneOut) Ask() string {
	return "Which word is not in the \"" + p.tag + "\" group?"
}

func (p *oddOneOut) Tiles() []string {
	t := make([]string, len(p.entries))
	for i, e := range p.entries {
		t[i] = e.Answers[0]
	}
	return t
}

// Odd returns the index of the odd tile, for tests.
func (p *oddOneOut) Odd() int { return p.odd }

func (p *oddOneOut) Check(a Attempt) Result {
	odd := p.entries[p.odd]
	r := Result{Result: words.Result{Tier: words.Miss, Expected: odd.Answers[0]}}
	if a.Pick == p.odd {
		r.Tier = words.Perfect
	}
	var group []string
	for _, e := range p.entries {
		if e.Tag == p.tag {
			group = append(group, e.Answers[0]+" = "+e.Prompt)
		}
	}
	r.Solution = []string{
		"Odd one: " + odd.Answers[0] + " = " + odd.Prompt + " (" + odd.Tag + ")",
		p.tag + ": " + strings.Join(group, ", "),
	}
	return r
}
