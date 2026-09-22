package words

import (
	"math/rand/v2"
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

func TestBlank(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 6))
	for _, s := range []string{"le chien", "l'ami", "canis", "ὁ ἵππος", "an madra", "tu"} {
		b := Blank(s, rng)
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
	if Blank("a", rng) != "a" {
		t.Error("one-letter words have nothing to hide")
	}
}
