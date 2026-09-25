package puzzle

import (
	"math/rand/v2"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/halpworld/halpwords/internal/words"
)

// made calls fn with many puzzles of kind k for every starter list and
// depth, and fails if none could be made.
func made(t *testing.T, k Kind, lock Lock, fn func(p Puzzle, lang *words.Language, depth int)) {
	t.Helper()
	seen := 0
	for lang, entries := range starter(t) {
		for depth := 1; depth <= 10; depth++ {
			rng := rand.New(rand.NewPCG(uint64(depth), uint64(k)))
			deck := words.NewDeck(entries, rng)
			for n := 0; n < 30; n++ {
				p := Make(k, lock, depth, deck, lang, lang.Defaults, rng)
				if p.Kind() != k {
					t.Fatalf("%s with %s words made %s", k, lang.Name, p.Kind())
				}
				seen++
				fn(p, lang, depth)
			}
		}
	}
	if seen == 0 {
		t.Fatalf("no %s puzzles made", k)
	}
}

func TestPairs(t *testing.T) {
	made(t, Pairs, Door, func(p Puzzle, lang *words.Language, depth int) {
		q := p.(*pairs)
		tiles, opts, start := p.Tiles(), q.Options(), q.Start()
		if len(tiles) != PairCount || len(opts) != PairCount || len(start) != PairCount || p.Word() != -1 {
			t.Fatalf("bad puzzle %+v", q)
		}
		for i := range tiles {
			if start[i] == i {
				t.Errorf("%q starts matched to %q", tiles[i], opts[i][start[i]])
			}
			for j := range i {
				if tiles[i] == tiles[j] || opts[0][i] == opts[0][j] {
					t.Fatalf("a word or meaning twice: %q %q", tiles, opts[0])
				}
			}
		}
		if slices.Sorted(slices.Values(start))[PairCount-1] != PairCount-1 || len(slices.Compact(slices.Sorted(slices.Values(start)))) != PairCount {
			t.Errorf("start %v is not a shuffle", start)
		}
		// Getting one pair wrong fails.
		c := solve(p).Choice
		c[0], c[1] = c[1], c[0]
		if r := p.Check(Attempt{Choice: c}); r.Passed() || !strings.Contains(r.Solution[2], "2 of 4") {
			t.Errorf("two pairs swapped: %v %q", r.Tier, r.Solution)
		}
	})
}

func TestPairsNeedFourWords(t *testing.T) {
	fr, _ := words.Lookup("fr")
	rng := rand.New(rand.NewPCG(1, 1))
	deck := words.NewDeck([]words.Entry{
		{Prompt: "dog", Answers: []string{"le chien"}},
		{Prompt: "hound", Answers: []string{"le chien"}}, // the same word
		{Prompt: "cat", Answers: []string{"le chat"}},
		{Prompt: "Dog", Answers: []string{"le toutou", "le chien"}}, // the same meaning
		{Prompt: "bird", Answers: []string{"l'oiseau"}},
	}, rng)
	for n := 0; n < 20; n++ {
		if k := Make(Pairs, Door, 1, deck, fr, fr.Defaults, rng).Kind(); k != Spell {
			t.Fatalf("made %s from three distinct words", k)
		}
	}
}

func TestTumbler(t *testing.T) {
	made(t, Tumbler, Chest, func(p Puzzle, lang *words.Language, depth int) {
		q := p.(*tumbler)
		wheels := 0
		for i, w := range q.Options() {
			if len(w) == 1 {
				continue
			}
			wheels++
			if len(w) != TumblerOptions(depth) || !slices.Contains(w, q.want[i]) {
				t.Fatalf("%q: wheel %q for %q", q.word, w, q.want[i])
			}
			for a := range w {
				for b := range a {
					if sameLetter(w[a], w[b]) {
						t.Fatalf("%q: wheel %q has a letter twice", q.word, w)
					}
				}
			}
			if s := w[q.Start()[i]]; base(s) == base(q.want[i]) {
				t.Errorf("%q: wheel starts on %q for %q", q.word, s, q.want[i])
			}
		}
		if wheels < MinWheels || wheels > MaxWheels {
			t.Errorf("%q has %d wheels", q.word, wheels)
		}
		if got := q.Spelled(solve(p).Choice); got != q.word {
			t.Errorf("right wheels spell %q, want %q", got, q.word)
		}
		if p.Clue() == "" || p.Word() < 0 {
			t.Errorf("%q: clue %q, word %d", q.word, p.Clue(), p.Word())
		}
	})
}

func TestTumblerFixedPlates(t *testing.T) {
	fr, _ := words.Lookup("fr")
	rng := rand.New(rand.NewPCG(2, 2))
	deck := words.NewDeck([]words.Entry{{Prompt: "bird", Answers: []string{"l'oiseau"}}}, rng)
	q := Make(Tumbler, Chest, 3, deck, fr, fr.Defaults, rng).(*tumbler)
	if o := q.Options(); len(o) != 7 || len(o[0]) != 1 || o[0][0] != "l'" {
		t.Fatalf("options %q", o)
	}
}

func TestCrossword(t *testing.T) {
	threes := 0
	made(t, Crossword, Chest, func(p Puzzle, lang *words.Language, depth int) {
		q := p.(*crossword)
		w, h, placed := q.Layout()
		if n := len(placed); n < 2 || n > 3 || (n == 3 && depth < Crossword3Depth) {
			t.Fatalf("%d words on floor %d", n, depth)
		}
		if len(placed) == 3 {
			threes++
			if d := placed[1].X - placed[2].X; d > -2 && d < 2 {
				t.Errorf("down words %d columns apart", d)
			}
		}
		// Every letter is on the grid, and where two words share a cell
		// they have the same letter.
		grid := map[[2]int]string{}
		for i, pl := range placed {
			letters := units(q.answers[i])
			if pl.Len != len(letters) || pl.Len < MinCross || pl.Len > MaxCross || pl.Down != (i > 0) {
				t.Fatalf("%q placed %+v", q.answers[i], pl)
			}
			if pl.Clue != q.entries[i].Prompt {
				t.Errorf("clue %q for %q", pl.Clue, q.entries[i].Prompt)
			}
			for k, l := range letters {
				x, y := pl.Cell(k)
				if x < 0 || y < 0 || x >= w || y >= h {
					t.Fatalf("%q off the %dx%d grid at %d,%d", q.answers[i], w, h, x, y)
				}
				if o, ok := grid[[2]int{x, y}]; ok && !sameLetter(o, l) {
					t.Errorf("%q and %q cross at %q", o, l, q.answers)
				}
				grid[[2]int{x, y}] = l
			}
		}
		if want := placed[0].Len + placed[1].Len - 1; len(placed) == 2 && len(grid) != want {
			t.Errorf("%q fill %d cells, want %d", q.answers, len(grid), want)
		}
		for _, a := range q.answers {
			if strings.ContainsAny(a, " '’") {
				t.Errorf("%q has an article or space", a)
			}
		}
	})
	if threes == 0 {
		t.Error("no three-word crosswords")
	}
}

func TestRiddleBank(t *testing.T) {
	bank := Riddles()
	for word, rs := range bank {
		whole := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(word) + `\b`)
		for _, r := range rs {
			if whole.MatchString(strings.ReplaceAll(r, "____", "")) {
				t.Errorf("riddle for %q gives it away: %q", word, r)
			}
			if n := utf8.RuneCountInString(r); n > 110 {
				t.Errorf("riddle for %q is %d characters: %q", word, n, r)
			}
		}
	}
	// Every starter word has a riddle.
	for lang, entries := range starter(t) {
		for _, e := range entries {
			if len(bank[strings.ToLower(e.Prompt)]) == 0 {
				t.Errorf("%s %q has no riddle", lang.Name, e.Prompt)
			}
		}
	}
}

func TestRiddle(t *testing.T) {
	made(t, Riddle, Door, func(p Puzzle, lang *words.Language, depth int) {
		e := starterEntry(t, lang, p.Word())
		if !slices.Contains(Riddles()[strings.ToLower(e.Prompt)], p.Clue()) {
			t.Errorf("riddle %q is not about %q", p.Clue(), e.Prompt)
		}
		// Every accepted answer opens it.
		for _, a := range e.Answers {
			if r := p.Check(Attempt{Text: a}); r.Tier != words.Perfect {
				t.Errorf("%q graded %v", a, r.Tier)
			}
		}
	})
	// Lists with no riddle words get spelling puzzles.
	fr, _ := words.Lookup("fr")
	rng := rand.New(rand.NewPCG(1, 1))
	deck := words.NewDeck([]words.Entry{{Prompt: "yes", Answers: []string{"oui"}}}, rng)
	if k := Make(Riddle, Door, 1, deck, fr, fr.Defaults, rng).Kind(); k != Spell {
		t.Errorf("riddle without a riddle word made %s", k)
	}
}

func starterEntry(t *testing.T, lang *words.Language, id int) words.Entry {
	t.Helper()
	return starter(t)[lang][id]
}
