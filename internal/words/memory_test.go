package words

import (
	"encoding/json"
	"math/rand/v2"
	"testing"
)

func TestLeitnerBoxes(t *testing.T) {
	m := NewMemory()
	e := Entry{Prompt: "dog", Answers: []string{"le chien"}}
	if m.Box(e) != 0 || m.IsDue(e) {
		t.Fatal("a new word has a box or is due")
	}
	m.Record(e, Answer{Tier: Correct})
	if m.Box(e) != 1 {
		t.Fatalf("first answer: box %d, want 1", m.Box(e))
	}
	for want := 2; want <= Boxes+1; want++ {
		m.Record(e, Answer{Tier: Perfect})
		if got := m.Box(e); got != min(want, Boxes) {
			t.Fatalf("perfect answer: box %d, want %d", got, min(want, Boxes))
		}
	}
	m.Record(e, Answer{Tier: Correct})
	if m.Box(e) != Boxes {
		t.Fatal("a correct answer moved the word")
	}
	m.Record(e, Answer{Tier: AccentSlip, Mistake: AccentMistake})
	if m.Box(e) != Boxes-1 {
		t.Fatal("a slip did not move the word down a box")
	}
	m.Record(e, Answer{Tier: Perfect, Hinted: true})
	if m.Box(e) != Boxes-2 {
		t.Fatalf("a hinted answer: box %d, want %d", m.Box(e), Boxes-2)
	}
	m.Record(e, Answer{Tier: Miss, Mistake: WrongWord})
	if m.Box(e) != 1 {
		t.Fatal("a miss did not send the word back to box 1")
	}
	c := m.Card(e)
	if c.Seen != 10 || c.Misses != 1 || c.Perfect != 5 || c.Mistakes[AccentMistake] != 1 {
		t.Fatalf("statistics %+v", c)
	}
}

func TestDueWords(t *testing.T) {
	m := NewMemory()
	a := Entry{Prompt: "a", Answers: []string{"x"}}
	b := Entry{Prompt: "b", Answers: []string{"y"}}
	m.Record(a, Answer{Tier: Perfect})
	if m.IsDue(a) {
		t.Fatal("due straight after an answer")
	}
	for i := 0; i < boxGap[1]; i++ {
		m.Record(b, Answer{Tier: Miss})
	}
	if !m.IsDue(a) {
		t.Fatal("not due after its box's gap")
	}
}

func TestMemoryKeepsTimesAndSurvivesJSON(t *testing.T) {
	m := NewMemory()
	e := Entry{Prompt: "cat", Answers: []string{"le chat", "la chatte"}}
	m.Record(e, Answer{Tier: Perfect, Timed: true, Secs: 2})
	m.Record(e, Answer{Tier: Graze, Timed: true, Secs: 4, Mistake: SwapMistake})
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var back Memory
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	// Extra answers in the list don't change the word's identity.
	c := back.Card(Entry{Prompt: "cat", Answers: []string{"le chat"}})
	if c == nil || c.AvgSecs() != 3 || c.Accuracy() != 0.5 || back.Clock != 2 {
		t.Fatalf("card after JSON: %+v", c)
	}
	s := back.Summarize([]Entry{e, {Prompt: "new", Answers: []string{"z"}}})
	if s.Words != 2 || s.InBox[0] != 1 || s.CommonMistake() != SwapMistake || s.AvgSecs() != 3 {
		t.Fatalf("summary %+v", s)
	}
}

func TestWeakest(t *testing.T) {
	m := NewMemory()
	es := []Entry{
		{Prompt: "good", Answers: []string{"a"}},
		{Prompt: "bad", Answers: []string{"b"}},
		{Prompt: "new", Answers: []string{"c"}},
		{Prompt: "ok", Answers: []string{"d"}},
	}
	for i := 0; i < 5; i++ {
		m.Record(es[0], Answer{Tier: Perfect})
	}
	m.Record(es[1], Answer{Tier: Miss})
	m.Record(es[1], Answer{Tier: Miss})
	m.Record(es[3], Answer{Tier: Perfect})
	m.Record(es[3], Answer{Tier: Perfect})
	got := m.Weakest(es, 5)
	if len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Fatalf("weakest %v, want [1 3]", got)
	}
}

func TestDeckDealsDueAndNewWords(t *testing.T) {
	var es []Entry
	for i := 0; i < 40; i++ {
		es = append(es, Entry{Prompt: string(rune('a'+i%26)) + string(rune('a'+i/26)), Answers: []string{"w"}})
	}
	m := NewMemory()
	// Words 0-9 are mastered, 10-19 are due, the rest are new.
	for i := 0; i < 20; i++ {
		m.Record(es[i], Answer{Tier: Perfect})
	}
	for i := 0; i < 20; i++ {
		c := m.Card(es[i])
		c.Box, c.Due = Boxes, m.Clock+1000
		if i >= 10 {
			c.Box, c.Due = 2, m.Clock
		}
	}
	d := NewDeck(es, rand.New(rand.NewPCG(1, 2)))
	d.SetMemory(m)
	count := map[string]int{}
	for i := 0; i < 1000; i++ {
		e, _ := d.Next()
		switch id := indexOf(es, e); {
		case id < 10:
			count["mastered"]++
		case id < 20:
			count["due"]++
		default:
			count["new"]++
		}
	}
	if count["due"] < 300 || count["new"] < 300 || count["mastered"] > 100 {
		t.Fatalf("deals %v: want mostly due and new words", count)
	}
}

func indexOf(es []Entry, e Entry) int {
	for i := range es {
		if es[i].Prompt == e.Prompt {
			return i
		}
	}
	return -1
}

func TestDeckAimsAtADifficulty(t *testing.T) {
	var es []Entry
	for i := 0; i < 25; i++ {
		a := "ab"
		if i%5 == 0 {
			a = "une bibliothèque"
		}
		es = append(es, Entry{Prompt: string(rune('a' + i)), Answers: []string{a}})
	}
	d := NewDeck(es, rand.New(rand.NewPCG(3, 4)))
	long := 0
	for i := 0; i < 400; i++ {
		e, _, _ := d.NextNear(18, nil)
		if len(e.Answers[0]) > 2 {
			long++
		}
	}
	// At random, one word in five is long; aiming high deals them often.
	if long < 150 {
		t.Fatalf("aiming at hard words dealt %d long words in 400", long)
	}
}

func TestDeckAnswerRecords(t *testing.T) {
	es := []Entry{{Prompt: "a", Answers: []string{"x"}}, {Prompt: "b", Answers: []string{"y"}}}
	d := NewDeck(es, rand.New(rand.NewPCG(1, 1)))
	m := NewMemory()
	d.SetMemory(m)
	d.Answer(1, Answer{Tier: Miss})
	if d.Review() != 1 || m.Box(es[1]) != 1 {
		t.Fatal("a miss was not recorded")
	}
	d.Answer(1, Answer{Tier: Perfect, Hinted: true})
	if d.Review() != 1 {
		t.Fatal("a hinted answer took the word off the review list")
	}
	d.Answer(1, Answer{Tier: Correct})
	if d.Review() != 0 || m.Card(es[1]).Seen != 3 {
		t.Fatal("a right answer was not recorded")
	}
	d.Answer(-1, Answer{Tier: Perfect}) // not about one word
}

func TestClassify(t *testing.T) {
	for _, tc := range []struct {
		typed, want string
		m           Mistake
	}{
		{"le chien", "le chien", NoMistake},
		{"Le Chien", "le chien", NoMistake},
		{"", "le chien", WrongWord},
		{"ecole", "école", AccentMistake},
		{"le chein", "le chien", SwapMistake},
		{"le chen", "le chien", MissingMistake},
		{"le chiien", "le chien", DoubleMistake},
		{"la pome", "la pomme", DoubleMistake},
		{"le chiens", "le chien", ExtraMistake},
		{"le chian", "le chien", LetterMistake},
		{"le chat", "le chien", WrongWord},
		{"le cheval", "le chien", WrongWord},
		{"l’oiseau", "l'oiseau", NoMistake},
		{"ecolle", "école", DoubleMistake},
	} {
		if got := Classify(tc.typed, tc.want, nil); got != tc.m {
			t.Errorf("Classify(%q, %q) = %v, want %v", tc.typed, tc.want, got, tc.m)
		}
	}
}

func TestClassifyWithoutArticle(t *testing.T) {
	fr, _ := Lookup("fr")
	if got := Classify("chein", "le chien", fr); got != SwapMistake {
		t.Fatalf("got %v, want swapped letters", got)
	}
	if got := Classify("la chien", "le chien", fr); got != LetterMistake {
		t.Fatalf("wrong article: got %v", got)
	}
}

func TestDifficulty(t *testing.T) {
	short := Difficulty(Entry{Answers: []string{"chat"}})
	marked := Difficulty(Entry{Answers: []string{"thé"}})
	greek := Difficulty(Entry{Answers: []string{"λογος"}})
	accented := Difficulty(Entry{Answers: []string{"λόγος"}})
	if short != 4 || marked != 4.5 || greek != 5 || accented != 6.5 {
		t.Fatalf("difficulties %v %v %v %v", short, marked, greek, accented)
	}
}
