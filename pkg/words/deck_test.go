package words

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

func testEntries(n int) []Entry {
	var es []Entry
	for i := 0; i < n; i++ {
		es = append(es, Entry{Prompt: string(rune('a' + i)), Answers: []string{"x"}})
	}
	return es
}

func TestDeckAvoidsRepeats(t *testing.T) {
	d := NewDeck(testEntries(20), rand.New(rand.NewPCG(1, 2)))
	var last []int
	for n := 0; n < 200; n++ {
		_, i := d.Next()
		for _, j := range last {
			if i == j {
				t.Fatalf("word %d repeated within %d deals", i, recentLen)
			}
		}
		last = append(last, i)
		if len(last) > 3 {
			last = last[1:]
		}
	}
}

func TestDeckSingleWord(t *testing.T) {
	d := NewDeck(testEntries(1), rand.New(rand.NewPCG(1, 2)))
	for n := 0; n < 5; n++ {
		if _, i := d.Next(); i != 0 {
			t.Fatalf("got %d", i)
		}
	}
}

func TestDeckReview(t *testing.T) {
	d := NewDeck(testEntries(50), rand.New(rand.NewPCG(3, 4)))
	_, missed := d.Next()
	d.Mark(missed, false)
	d.Mark(missed, false) // counted once
	if d.Review() != 1 {
		t.Fatalf("review = %d, want 1", d.Review())
	}
	seen := 0
	for n := 0; n < 100; n++ {
		if _, i := d.Next(); i == missed {
			seen++
		}
	}
	// Without review a word comes up about 2 times in 100.
	if seen < 10 {
		t.Errorf("missed word came back %d times in 100 deals", seen)
	}
	d.Mark(missed, true)
	if d.Review() != 0 {
		t.Errorf("review = %d after a correct answer", d.Review())
	}
}

func TestDeckNextWhere(t *testing.T) {
	d := NewDeck(testEntries(20), rand.New(rand.NewPCG(5, 6)))
	even := func(e Entry) bool { return (e.Prompt[0]-'a')%2 == 0 }
	d.Mark(1, false) // odd: must never be dealt, though it is up for review
	d.Mark(4, false)
	fours := 0
	for n := 0; n < 100; n++ {
		e, i, ok := d.NextWhere(even)
		if !ok || !even(e) || d.Entries()[i].Prompt != e.Prompt {
			t.Fatalf("dealt %v (%d, %v)", e, i, ok)
		}
		if i == 4 {
			fours++
		}
	}
	if fours < 10 {
		t.Errorf("missed word came back %d times in 100 deals", fours)
	}
	if _, i, ok := d.NextWhere(func(Entry) bool { return false }); ok || i != -1 {
		t.Errorf("no word fits, got %d, %v", i, ok)
	}
}

func TestBlank(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 6))
	for _, s := range []string{"le chien", "l'ami", "canis", "ὁ ἵππος", "an madra", "tu"} {
		b := Blank(s, 0.4, rng)
		if utf8.RuneCountInString(b) != utf8.RuneCountInString(s) {
			t.Errorf("Blank(%q) = %q changed length", s, b)
		}
		if !strings.Contains(b, "_") {
			t.Errorf("Blank(%q) = %q hides nothing", s, b)
		}
		br, sr := []rune(b), []rune(s)
		for i := range sr {
			if br[i] != '_' && br[i] != sr[i] {
				t.Errorf("Blank(%q) = %q changed a letter", s, b)
			}
			if br[i] == '_' && (i == 0 || sr[i-1] == ' ' || sr[i-1] == '\'') {
				t.Errorf("Blank(%q) = %q hid a first letter", s, b)
			}
		}
	}
	if Blank("a", 0.4, rng) != "a" {
		t.Error("one-letter words have nothing to hide")
	}
}

func TestBlankShare(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 8))
	for _, c := range []struct {
		share float64
		want  int
	}{{0, 1}, {0.3, 3}, {0.6, 6}, {1, 10}} {
		b := Blank("abcdefghijk", c.share, rng) // ten letters can be hidden
		if n := strings.Count(b, "_"); n != c.want {
			t.Errorf("share %v hid %d letters (%q), want %d", c.share, n, b, c.want)
		}
	}
}

func TestDeckState(t *testing.T) {
	d := NewDeck(testEntries(10), rand.New(rand.NewPCG(1, 2)))
	for n := 0; n < 8; n++ {
		_, i := d.Next()
		d.Mark(i, n%3 != 0)
	}
	s := d.State()
	if len(s.Recent) == 0 || len(s.Review) == 0 {
		t.Fatalf("state %+v is missing words", s)
	}
	e := NewDeck(testEntries(10), rand.New(rand.NewPCG(1, 2)))
	e.SetState(s)
	if e.Review() != d.Review() || !slices.Equal(e.recent, d.recent) {
		t.Fatalf("got %+v, want %+v", e.State(), s)
	}

	// A shorter word list drops the words that are gone.
	short := NewDeck(testEntries(2), rand.New(rand.NewPCG(1, 2)))
	short.SetState(DeckState{Recent: []int{0, 5, 1}, Review: []int{9, 1, -1}})
	if got := short.State(); !slices.Equal(got.Recent, []int{0, 1}) || !slices.Equal(got.Review, []int{1}) {
		t.Fatalf("got %+v", got)
	}
}
