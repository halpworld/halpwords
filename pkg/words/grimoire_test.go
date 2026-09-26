package words

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// gameGrimoire is the game's Grimoire scene (internal/scene/grimoire.go)
// as it was before its order, percentages and "watch out for" moved
// here, copied line for line as far as the words and text it draws.
// NewGrimoire must give the same.
type gameGrimoire struct {
	entries []Entry
	order   []int
	sum     Summary
}

func gameLoad(mem *Memory, all []Entry, by GrimoireSort) *gameGrimoire {
	g := &gameGrimoire{}
	seen := map[string]bool{}
	for _, e := range all {
		if k := Key(e); !seen[k] {
			seen[k] = true
			g.entries = append(g.entries, e)
		}
	}
	g.sum = mem.Summarize(g.entries)
	g.order = make([]int, len(g.entries))
	for i := range g.order {
		g.order[i] = i
	}
	switch by {
	case GrimoireWeakest:
		weak := mem.Weakest(g.entries, len(g.entries))
		rank := map[int]int{}
		for r, i := range weak {
			rank[i] = r + 1
		}
		sort.SliceStable(g.order, func(a, b int) bool {
			ia, ib := g.order[a], g.order[b]
			ra, rb := rank[ia], rank[ib]
			switch {
			case ra > 0 && rb > 0:
				return ra < rb
			case ra > 0 || rb > 0:
				return ra > 0
			}
			ba, bb := mem.Box(g.entries[ia]), mem.Box(g.entries[ib])
			return ba > bb
		})
	case GrimoireAZ:
		sort.SliceStable(g.order, func(a, b int) bool {
			return strings.ToLower(g.entries[g.order[a]].Prompt) < strings.ToLower(g.entries[g.order[b]].Prompt)
		})
	}
	return g
}

func (g *gameGrimoire) head() string {
	s := g.sum
	head := fmt.Sprintf("%d of %d words mastered", s.Mastered(), s.Words)
	if s.Seen > 0 {
		head += fmt.Sprintf("   ·   right %d%%", int(s.Accuracy()*100+0.5))
	}
	return head
}

func (g *gameGrimoire) rows(mem *Memory) [][]string {
	var out [][]string
	for _, i := range g.order {
		e := g.entries[i]
		c := mem.Card(e)
		box := 0
		if c != nil {
			box = c.Box
		}
		row := []string{e.Prompt, e.Answers[0], fmt.Sprint(box), "new"}
		if c != nil {
			row[3] = fmt.Sprintf("%d%%", int(c.Accuracy()*100+0.5))
		}
		var text string
		switch {
		case c == nil:
			text = "Not met yet."
		case c.Box == Boxes:
			text = fmt.Sprintf("Mastered! Answered %d times.", c.Seen)
		default:
			text = fmt.Sprintf("Answered %d times, %d perfect, %d missed.", c.Seen, c.Perfect, c.Misses)
			worst, n := NoMistake, 0
			for m, k := range c.Mistakes {
				if k > n {
					worst, n = Mistake(m), k
				}
			}
			if worst != NoMistake {
				text += " Watch out for " + worst.String() + "."
			}
		}
		out = append(out, append(row, text))
	}
	return out
}

// ours draws the same lines from NewGrimoire.
func ours(g *Grimoire) (string, [][]string) {
	s := g.Summary
	head := fmt.Sprintf("%d of %d words mastered", s.Mastered(), s.Words)
	if s.Seen > 0 {
		head += fmt.Sprintf("   ·   right %d%%", g.Right())
	}
	var rows [][]string
	for _, r := range g.Rows {
		row := []string{r.Entry.Prompt, r.Entry.Answers[0], fmt.Sprint(r.Box), "new", ""}
		if r.Met() {
			row[3] = fmt.Sprintf("%d%%", r.Right)
		}
		switch {
		case !r.Met():
			row[4] = "Not met yet."
		case r.Mastered():
			row[4] = fmt.Sprintf("Mastered! Answered %d times.", r.Card.Seen)
		default:
			row[4] = fmt.Sprintf("Answered %d times, %d perfect, %d missed.", r.Card.Seen, r.Card.Perfect, r.Card.Misses)
			if r.Watch != NoMistake {
				row[4] += " Watch out for " + r.Watch.String() + "."
			}
		}
		rows = append(rows, row)
	}
	return head, rows
}

func TestGrimoireIsTheScenes(t *testing.T) {
	e := func(p, a string) Entry { return Entry{Prompt: p, Answers: []string{a}} }
	butterfly, cat, coffee, dog, owl := e("butterfly", "papillon"), e("Cat", "chat"), e("coffee", "café"), e("dog", "chien"), e("Owl", "hibou")
	apple := e("apple", "pomme")
	perfect := Answer{Tier: Perfect}
	mem := NewMemory()
	for range 5 {
		mem.Record(butterfly, perfect)
	}
	mem.Record(cat, perfect)
	mem.Record(cat, Answer{Tier: Miss, Mistake: WrongWord})
	mem.Record(cat, Answer{Tier: AccentSlip, Mistake: AccentMistake})
	mem.Record(cat, Answer{Tier: AccentSlip, Mistake: AccentMistake})
	mem.Record(coffee, Answer{Tier: Correct})
	mem.Record(dog, perfect)
	mem.Record(dog, perfect)
	mem.Record(dog, Answer{Tier: Graze, Mistake: SwapMistake})
	// A tie: the first kind wins.
	mem.Record(owl, Answer{Tier: Graze, Mistake: LetterMistake})
	mem.Record(owl, Answer{Tier: Graze, Mistake: DoubleMistake})
	// A word twice (another list) and one not met.
	entries := []Entry{dog, apple, butterfly, cat, owl, coffee, cat}
	for _, mem := range []*Memory{NewMemory(), mem} {
		for by := range NumGrimoireSorts {
			game := gameLoad(mem, entries, by)
			g := mem.NewGrimoire(entries, by)
			head, rows := ours(g)
			if want := game.head(); head != want {
				t.Errorf("%s: head = %q, the scene's %q", by, head, want)
			}
			if want := game.rows(mem); !reflect.DeepEqual(rows, want) {
				t.Errorf("%s: rows = %q, the scene's %q", by, rows, want)
			}
			if g.Summary != game.sum || g.Sort != by {
				t.Errorf("%s: summary = %+v, the scene's %+v", by, g.Summary, game.sum)
			}
			if order := mem.SortGrimoire(Distinct(entries), by); !reflect.DeepEqual(order, game.order) {
				t.Errorf("%s: SortGrimoire = %v, the scene's %v", by, order, game.order)
			}
		}
	}
	if w := mem.Card(owl).WatchOut(); w != DoubleMistake {
		t.Errorf("owl: WatchOut = %v, want the first kind on a tie", w)
	}
	if w := mem.Card(apple).WatchOut(); w != NoMistake {
		t.Errorf("a nil card's WatchOut = %v", w)
	}
}

func TestGrimoireSorts(t *testing.T) {
	for s, want := range map[GrimoireSort][2]string{
		GrimoireWeakest:   {"weakest first", "weakest"},
		GrimoireListOrder: {"list order", "list"},
		GrimoireAZ:        {"A to Z", "az"},
	} {
		if s.String() != want[0] || s.ID() != want[1] {
			t.Errorf("%d: %q %q, want %q", s, s.String(), s.ID(), want)
		}
		if got, ok := ParseGrimoireSort(want[1]); !ok || got != s {
			t.Errorf("ParseGrimoireSort(%q) = %v, %v", want[1], got, ok)
		}
	}
	if _, ok := ParseGrimoireSort("best"); ok {
		t.Error("ParseGrimoireSort accepted an unknown sort")
	}
	for share, want := range map[float64]int{0: 0, 0.125: 13, 0.5: 50, 2.0 / 3: 67, 0.994: 99, 0.995: 100, 1: 100} {
		if got := Percent(share); got != want {
			t.Errorf("Percent(%v) = %d, want %d", share, got, want)
		}
	}
}
