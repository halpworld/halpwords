package puzzle

import (
	"math/rand/v2"
	"slices"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/halpworld/halpwords/assets"
	"github.com/halpworld/halpwords/internal/words"
)

// starter returns the starter lists' words for each language.
func starter(t *testing.T) map[*words.Language][]words.Entry {
	t.Helper()
	lists, err := words.LoadFS(assets.Words, "words")
	if err != nil {
		t.Fatal(err)
	}
	out := map[*words.Language][]words.Entry{}
	for _, l := range lists {
		lang, _ := words.Lookup(l.Language)
		out[lang] = append(out[lang], l.Entries...)
	}
	return out
}

// each calls fn with many puzzles for every starter list, lock and depth.
func each(t *testing.T, fn func(p Puzzle, lang *words.Language, lock Lock, depth int)) {
	for lang, entries := range starter(t) {
		for _, lock := range []Lock{Door, Chest} {
			for depth := 1; depth <= 10; depth++ {
				rng := rand.New(rand.NewPCG(uint64(depth), uint64(lock)))
				deck := words.NewDeck(entries, rng)
				for n := 0; n < 40; n++ {
					fn(New(lock, depth, deck, lang, rng), lang, lock, depth)
				}
			}
		}
	}
}

// solve returns the right answer to p.
func solve(p Puzzle) Attempt {
	switch q := p.(type) {
	case *oddOneOut:
		return Attempt{Pick: q.Odd()}
	case *pairs:
		a := Attempt{}
		for i := range q.entries {
			a.Choice = append(a.Choice, i)
		}
		return a
	case *tumbler:
		a := Attempt{}
		for i, w := range q.slots {
			a.Choice = append(a.Choice, slices.Index(w, q.want[i]))
		}
		return a
	case *crossword:
		return Attempt{Texts: q.answers}
	}
	return Attempt{Text: p.Check(Attempt{}).Expected}
}

func TestRightAnswerOpens(t *testing.T) {
	each(t, func(p Puzzle, lang *words.Language, lock Lock, depth int) {
		right := solve(p)
		if r := p.Check(right); r.Tier != words.Perfect || !r.Passed() || len(r.Solution) == 0 {
			t.Errorf("%s %q: right answer %+v graded %v", p.Kind(), p.Clue(), right, r.Tier)
		}
		if p.Answer() != Pick && p.Check(Attempt{}).Passed() {
			t.Errorf("%s %q: empty answer passed", p.Kind(), p.Clue())
		}
		switch p.Answer() {
		case Pick:
			if p.Check(Attempt{Pick: (right.Pick + 1) % len(p.Tiles())}).Passed() {
				t.Errorf("%s: wrong pick passed", p.Kind())
			}
			return
		case Match, Dial:
			if start := p.(Chooser).Start(); p.Check(Attempt{Choice: start}).Passed() {
				t.Errorf("%s: passed as it starts, %v", p.Kind(), start)
			}
			return
		case Grid:
			wrong := slices.Clone(right.Texts)
			wrong[len(wrong)-1] = "qqqqqq"
			if p.Check(Attempt{Texts: wrong}).Passed() {
				t.Errorf("crossword with a wrong word passed")
			}
			return
		}
		r := p.Check(right)
		if !strings.Contains(r.Solution[0], right.Text) {
			t.Errorf("%s: solution %q does not show %q", p.Kind(), r.Solution, right.Text)
		}
		if p.Check(Attempt{Text: "qqqqqq"}).Passed() {
			t.Errorf("%s: nonsense passed", p.Kind())
		}
		if p.Word() < 0 {
			t.Errorf("%s: typed puzzle has no word to mark", p.Kind())
		}
		if (p.Answer() == Native) != (p.Kind() == Reverse) {
			t.Errorf("%s answered in %v", p.Kind(), p.Answer())
		}
	})
}

func TestAnagramTiles(t *testing.T) {
	seen := 0
	each(t, func(p Puzzle, lang *words.Language, lock Lock, depth int) {
		if p.Kind() != Anagram {
			return
		}
		seen++
		word := p.Check(Attempt{}).Expected
		_, rest := splitArticle(word, lang)
		want := Letters(rest)
		got := append([]string(nil), p.Tiles()...)
		extra := len(got) - len(want)
		if extra < 0 || extra > 1 || (extra == 1 && (lock != Chest || depth < DecoyDepth)) {
			t.Fatalf("%q: tiles %q for letters %q", word, got, want)
		}
		if strings.Join(got[:len(want)], "") == strings.Join(want, "") && extra == 0 {
			t.Errorf("%q: tiles %q are not scrambled", word, got)
		}
		sort.Strings(got)
		for _, l := range want {
			i := sort.SearchStrings(got, l)
			if i == len(got) || got[i] != l {
				t.Fatalf("%q: tiles %q lack %q", word, p.Tiles(), l)
			}
			got = append(got[:i], got[i+1:]...)
		}
		// The blanks show where the letters go.
		_, pattern, _ := strings.Cut(p.Clue(), "→  ")
		if strings.Count(pattern, "_") != len(want) {
			t.Errorf("%q: pattern %q", word, pattern)
		}
	})
	if seen == 0 {
		t.Fatal("no anagrams made")
	}
}

func TestMissingPattern(t *testing.T) {
	each(t, func(p Puzzle, lang *words.Language, lock Lock, depth int) {
		if p.Kind() != Missing {
			return
		}
		word := p.Check(Attempt{}).Expected
		_, blank, ok := strings.Cut(p.Clue(), "→  ")
		if !ok || !strings.Contains(blank, "_") || utf8.RuneCountInString(blank) != utf8.RuneCountInString(word) {
			t.Fatalf("%q: clue %q", word, p.Clue())
		}
		b, w := []rune(blank), []rune(word)
		for i := range w {
			if b[i] != '_' && b[i] != w[i] {
				t.Fatalf("%q: clue %q changes a letter", word, p.Clue())
			}
		}
	})
}

func TestOddOneOut(t *testing.T) {
	seen := 0
	each(t, func(p Puzzle, lang *words.Language, lock Lock, depth int) {
		o, ok := p.(*oddOneOut)
		if !ok {
			return
		}
		seen++
		tiles := p.Tiles()
		if len(tiles) != OddGroup+1 || p.Word() != -1 || p.Clue() != "" {
			t.Fatalf("bad puzzle %+v", o)
		}
		odd := 0
		for i, e := range o.entries {
			for j := range i {
				if tiles[i] == tiles[j] {
					t.Fatalf("tile %q twice", tiles[i])
				}
			}
			if e.Tag != o.tag {
				odd++
			}
		}
		if odd != 1 || o.entries[o.Odd()].Tag == o.tag {
			t.Fatalf("%d odd words in %q (%s)", odd, tiles, o.tag)
		}
	})
	if seen == 0 {
		t.Fatal("no odd-one-out puzzles made")
	}
}

func TestReverseAcceptsEveryMeaning(t *testing.T) {
	all := []words.Entry{
		{Prompt: "present", Answers: []string{"le cadeau"}},
		{Prompt: "gift", Answers: []string{"le don", "le cadeau"}},
		{Prompt: "dog", Answers: []string{"le chien"}},
	}
	p := newReverse(all[0], 0, all)
	for _, typed := range []string{"present", "gift", "a gift", "the present"} {
		if r := p.Check(Attempt{Text: typed}); r.Tier != words.Perfect {
			t.Errorf("%q graded %v", typed, r.Tier)
		}
	}
	if p.Check(Attempt{Text: "dog"}).Passed() {
		t.Error("dog passed")
	}
}

func TestKinds(t *testing.T) {
	for depth := 1; depth <= 10; depth++ {
		for _, k := range Kinds(Door, depth) {
			if k == Missing || k == Tumbler || k == Crossword {
				t.Errorf("doors have %s puzzles on floor %d", k, depth)
			}
		}
		for _, k := range Kinds(Chest, depth) {
			if k == Reverse || k == OddOneOut || k == Pairs || k == Riddle {
				t.Errorf("chests have %s puzzles on floor %d", k, depth)
			}
		}
	}
	// Every kind turns up by floor 5.
	seen := map[Kind]bool{}
	for lang, entries := range starter(t) {
		rng := rand.New(rand.NewPCG(9, 9))
		deck := words.NewDeck(entries, rng)
		for n := 0; n < 60; n++ {
			seen[New(Door, 5, deck, lang, rng).Kind()] = true
			seen[New(Chest, 5, deck, lang, rng).Kind()] = true
		}
	}
	for k := Reverse; k <= Crossword; k++ {
		if !seen[k] {
			t.Errorf("no %s puzzles on floor 5", k)
		}
	}
}

// Lists without "## tag" groups cannot make odd-one-out puzzles, so they
// get spelling puzzles instead.
func TestFallbacks(t *testing.T) {
	fr, _ := words.Lookup("fr")
	rng := rand.New(rand.NewPCG(1, 1))
	deck := words.NewDeck([]words.Entry{
		{Prompt: "dog", Answers: []string{"le chien"}},
		{Prompt: "yes", Answers: []string{"oui"}},
		{Prompt: "in", Answers: []string{"en"}},
	}, rng)
	if k := Make(OddOneOut, Door, 1, deck, fr, rng).Kind(); k != Spell {
		t.Errorf("odd one out without groups made %s", k)
	}
	for n := 0; n < 20; n++ {
		p := Make(Anagram, Door, 1, deck, fr, rng)
		if w := p.Check(Attempt{}).Expected; w == "en" && p.Kind() != Spell {
			t.Errorf("anagram of %q made %s", w, p.Kind())
		}
	}
}

func TestLetters(t *testing.T) {
	for s, want := range map[string]string{
		"chien":  "c h i e n",
		"l'ami":  "l a m i",
		"λόγος":  "λ ό γ ο σ",
		"cailín": "c a i l í n",
		"été":   "é t é",
	} {
		if got := strings.Join(Letters(s), " "); got != want {
			t.Errorf("Letters(%q) = %q, want %q", s, got, want)
		}
	}
}
