package words

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
)

// goldenDeal deals a fixed number of words from a normal (nothing too long)
// list with a seeded rng, memory, misses, NextWhere and NextNear mixed in.
func goldenDeal() string {
	var es []Entry
	for i := 0; i < 12; i++ {
		es = append(es, Entry{Prompt: fmt.Sprintf("p%d", i), Answers: []string{strings.Repeat("ab", i%5+1)}})
	}
	d := NewDeck(es, rand.New(rand.NewPCG(7, 11)))
	d.SetMemory(NewMemory())
	var sb strings.Builder
	for n := 0; n < 60; n++ {
		var i int
		switch n % 3 {
		case 0:
			_, i = d.Next()
		case 1:
			_, i, _ = d.NextNear(3, nil)
		default:
			_, i, _ = d.NextWhere(func(e Entry) bool { return e.Prompt != "p3" })
		}
		tier := Correct
		if n%4 == 0 {
			tier = Miss
		}
		d.Answer(i, Answer{Tier: tier})
		fmt.Fprintf(&sb, "%d,", i)
	}
	return sb.String()
}

const goldenDealWant = "4,2,1,11,10,7,4,6,10,2,11,1,7,10,4,6,5,11,1,7,10,5,6,8,11,10,9,4,0,6,5,11,1,0,2,4,8,1,9,3,10,8,2,11,10,1,0,7,10,5,2,9,1,11,8,10,6,7,2,9,"

func TestQA3DealOrderUnchangedForNormalList(t *testing.T) {
	if got := goldenDeal(); got != goldenDealWant {
		t.Fatalf("dealing order changed:\n got %s\nwant %s", got, goldenDealWant)
	}
}

func runes(n int) string { return strings.Repeat("a", n) }

func TestQA3TooLongBoundary(t *testing.T) {
	const decomposed = "é" // two runes, one letter
	cases := []struct {
		name string
		ans  string
		want bool
	}{
		{"59 ascii", runes(59), false},
		{"60 ascii", runes(60), false},
		{"61 ascii", runes(61), true},
		{"60 multibyte (2-byte)", strings.Repeat("é", 60), false},
		{"61 multibyte (2-byte)", strings.Repeat("é", 61), true},
		{"60 CJK (3-byte)", strings.Repeat("世", 60), false},
		{"61 emoji (4-byte)", strings.Repeat("\U0001F600", 61), true},
		{"60 decomposed letters (120 runes)", strings.Repeat(decomposed, 60), false},
		{"61 decomposed letters", strings.Repeat(decomposed, 61), true},
		{"59 letters + 1 decomposed", runes(59) + decomposed, false},
		{"60 letters + 1 decomposed", runes(60) + decomposed, true},
		{"polytonic decomposed 60", strings.Repeat("ά", 60), false}, // alpha + acute composes to ά
		{"polytonic decomposed 61", strings.Repeat("ά", 61), true},
		{"empty", "", false},
	}
	for _, c := range cases {
		if got := TooLong(Entry{Answers: []string{c.ans}}); got != c.want {
			t.Errorf("%s: TooLong = %v, want %v", c.name, got, c.want)
		}
	}
	// Only the first answer (the shown solution) counts.
	if TooLong(Entry{Answers: []string{"ok", runes(100)}}) {
		t.Error("a long alternative made the entry too long")
	}
	if !TooLong(Entry{Answers: []string{runes(100), "ok"}}) {
		t.Error("a long first answer did not make the entry too long")
	}
}

func TestQA3MixedListNeverDealsTooLong(t *testing.T) {
	var es []Entry
	for i := 0; i < 20; i++ {
		a := "x"
		if i%3 == 0 {
			a = runes(MaxAnswerRunes + 1 + i)
		}
		es = append(es, Entry{Prompt: fmt.Sprintf("p%d", i), Answers: []string{a}})
	}
	for _, withMem := range []bool{false, true} {
		d := NewDeck(es, rand.New(rand.NewPCG(3, 4)))
		if withMem {
			d.SetMemory(NewMemory())
		}
		for n := 0; n < 2000; n++ {
			var e Entry
			var i int
			var ok bool
			switch n % 3 {
			case 0:
				e, i = d.Next()
				ok = true
			case 1:
				e, i, ok = d.NextNear(float64(n%7), nil)
			default:
				e, i, ok = d.NextWhere(func(Entry) bool { return true })
			}
			if !ok || TooLong(e) || i < 0 || i%3 == 0 {
				t.Fatalf("n=%d mem=%v dealt %q id %d ok=%v", n, withMem, e.Prompt, i, ok)
			}
			tier := Correct
			if n%2 == 0 {
				tier = Miss
			}
			d.Answer(i, Answer{Tier: tier})
		}
	}
}

func TestQA3AllTooLongStillDealsEverything(t *testing.T) {
	es := []Entry{longEntry("a"), longEntry("b"), longEntry("c")}
	d := NewDeck(es, rand.New(rand.NewPCG(1, 2)))
	seen := map[int]bool{}
	for n := 0; n < 200; n++ {
		e, i, ok := d.NextNear(-1, nil)
		if !ok || i < 0 || e.Prompt != es[i].Prompt {
			t.Fatalf("n=%d ok=%v id=%d", n, ok, i)
		}
		seen[i] = true
		d.Answer(i, Answer{Tier: Miss})
	}
	if len(seen) != 3 {
		t.Errorf("dealt only %v", seen)
	}
	// A filter that accepts nothing still reports false.
	if _, _, ok := d.NextWhere(func(Entry) bool { return false }); ok {
		t.Error("NextWhere dealt with an empty filter")
	}
}

func TestQA3SingleEntryDecks(t *testing.T) {
	d := NewDeck([]Entry{{Prompt: "only", Answers: []string{"x"}}}, rand.New(rand.NewPCG(1, 2)))
	if _, i := d.Next(); i != 0 {
		t.Fatalf("id %d", i)
	}
	d = NewDeck([]Entry{{Prompt: "only", Answers: []string{runes(80)}}}, rand.New(rand.NewPCG(1, 2)))
	if _, i := d.Next(); i != 0 {
		t.Fatalf("all-long single entry: id %d", i)
	}
}

// A missed too-long entry (from a save made before the change) must neither
// be dealt nor make Next spin; the good entries keep coming.
func TestQA3ReviewQueueWithTooLongEntryDoesNotLoop(t *testing.T) {
	es := append(testEntries(4), longEntry("long")) // id 4 is too long
	d := NewDeck(es, rand.New(rand.NewPCG(5, 6)))
	d.SetMemory(NewMemory())
	d.SetState(DeckState{Recent: []int{4}, Review: []int{4}})
	if d.Review() != 1 {
		t.Fatalf("review = %d", d.Review())
	}
	for n := 0; n < 500; n++ {
		_, i := d.Next()
		if i == 4 || i < 0 {
			t.Fatalf("n=%d dealt id %d", n, i)
		}
		d.Answer(i, Answer{Tier: Correct})
	}
	// Missed lists it still; callers get an id that indexes Entries.
	if m := d.Missed(); len(m) != 1 || m[0] != 4 || d.Entries()[m[0]].Prompt != "long" {
		t.Errorf("Missed = %v", m)
	}
}

// Every good word is in the review queue alongside a too-long one: the
// review retry must pick the good ones.
func TestQA3ReviewRetriesSkipTooLong(t *testing.T) {
	es := append(testEntries(3), longEntry("long"))
	d := NewDeck(es, rand.New(rand.NewPCG(9, 9)))
	d.SetState(DeckState{Review: []int{3, 1}})
	hits := 0
	for n := 0; n < 300; n++ {
		_, i := d.Next()
		if i == 3 {
			t.Fatal("dealt the too-long word from review")
		}
		if i == 1 {
			hits++
		}
		// keep the queue as it was
		d.SetState(DeckState{Review: []int{3, 1}})
	}
	if hits == 0 {
		t.Error("the good missed word was never retried")
	}
}

// A saved deck from before the change: ids index the whole list, including
// entries that are now too long; it loads without dropping them.
func TestQA3OldSavedDeckStillLoads(t *testing.T) {
	es := append(testEntries(3), longEntry("long1"), longEntry("long2"))
	d := NewDeck(es, rand.New(rand.NewPCG(1, 2)))
	d.SetState(DeckState{Recent: []int{4, 0, 3}, Review: []int{3, 1, 4}})
	got := d.State()
	if fmt.Sprint(got.Recent) != "[4 0 3]" || fmt.Sprint(got.Review) != "[3 1 4]" {
		t.Fatalf("state %+v", got)
	}
	// An id past the end (the list shrank) is still dropped.
	d.SetState(DeckState{Review: []int{1, 9, -1}})
	if fmt.Sprint(d.State().Review) != "[1]" {
		t.Fatalf("state %+v", d.State())
	}
	// And dealing from it works.
	for n := 0; n < 50; n++ {
		if _, i := d.Next(); i < 0 || i > 2 {
			t.Fatalf("dealt %d", i)
		}
	}
}

// Parse accepts a long answer; it is the deck that skips it.
func TestQA3ParseKeepsLongAnswer(t *testing.T) {
	long := runes(MaxAnswerRunes + 5)
	l, err := Parse(strings.NewReader("language: fr\nshort = ok\nlong = "+long+"\n"), "t.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Entries) != 2 {
		t.Fatalf("entries %d", len(l.Entries))
	}
	if tl := l.TooLong(); len(tl) != 1 || tl[0].Answers[0] != long {
		t.Fatalf("TooLong = %v", tl)
	}
}
