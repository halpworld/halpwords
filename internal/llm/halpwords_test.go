package llm

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/halpworld/halpwords/pkg/gameai"
	"github.com/halpworld/halpwords/pkg/words"
)

// fakeHalpwords is Halpwords AI answering from recorded replies.
type fakeHalpwords struct {
	available bool
	replies   map[string]string // by task
	err       error
	asked     []string
	reqs      []any
}

func (f *fakeHalpwords) Available() bool { return f.available }

func (f *fakeHalpwords) Do(_ context.Context, task string, req, out any) error {
	f.asked = append(f.asked, task)
	f.reqs = append(f.reqs, req)
	if f.err != nil {
		return f.err
	}
	return json.Unmarshal([]byte(f.replies[task]), out)
}

func TestHalpwordsChoice(t *testing.T) {
	store := newMemStore()
	s := Load(store)
	s.IgnoreEnvironment()
	if s.Ready() || s.Chosen() != Off {
		t.Fatal("ready with nothing set up")
	}
	h := &fakeHalpwords{}
	s.UseHalpwords(h)
	if s.Ready() || s.UsingHalpwords() {
		t.Fatal("Halpwords AI used while not available")
	}
	// A linked game on a plan with AI uses it with nothing set up.
	h.available = true
	if !s.Ready() || !s.UsingHalpwords() || s.Chosen() != Halpwords || s.Provider() != nil {
		t.Fatal("Halpwords AI not used when available")
	}
	if s.ForgeReady() {
		t.Fatal("the Word Forge offered with Halpwords AI")
	}
	if _, err := s.Ask(context.Background(), true, "", "hi", 10); !errors.Is(err, ErrNeedsKey) {
		t.Fatalf("Ask with Halpwords AI: %v", err)
	}
	// Choosing no AI turns it off, and that is kept.
	s.SetProvider(Off)
	if s.Ready() || s.Chosen() != Off {
		t.Fatal("Halpwords AI used after choosing no AI")
	}
	s2 := Load(store)
	s2.UseHalpwords(h)
	if s2.Ready() {
		t.Fatal("no AI was not kept")
	}
	s2.SetProvider(Halpwords)
	s3 := Load(store)
	s3.UseHalpwords(h)
	if !s3.Ready() || s3.Chosen() != Halpwords {
		t.Fatal("choosing Halpwords AI was not kept")
	}
	h.available = false
	if s3.Ready() || !errors.Is(s3.ready(), ErrNoHalpwords) || s3.Chosen() != Halpwords {
		t.Fatalf("chosen but not available: %v", s3.ready())
	}
	// A provider with a key comes first when chosen.
	h.available = true
	s3.SetProvider(Anthropic)
	if s3.UsingHalpwords() {
		t.Fatal("Halpwords AI used with a provider chosen")
	}
}

func TestHalpwordsContent(t *testing.T) {
	s := Load(nil)
	h := &fakeHalpwords{available: true, replies: map[string]string{
		gameai.Director: `{"name":"The Drowned Pantry","theme":0,"intro":"Water drips.","lore":["Beware."],"monsters":{"Cave Bat":"Crumb Bat"},"boss":""}`,
		gameai.Cloze:    `{"items":[{"n":1,"cloze":"Je promène ___ au parc.","cloze_en":"I walk the dog in the park."}]}`,
		gameai.Riddles:  `{"items":[{"n":2,"riddle":"I purr on your lap."},{"n":1,"riddle":"I am a dog."}]}`,
		gameai.Taunts:   `{"taunts":[{"text":"Ton pain est à moi !","english":"Your bread is mine!"}]}`,
		gameai.Insight:  `{"tips":[{"n":1,"tip":"Chien: she and her dog."}]}`,
	}}
	s.UseHalpwords(h)
	ctx := context.Background()
	entries := []words.Entry{
		{Prompt: "dog", Answers: []string{"le chien"}},
		{Prompt: "cat", Answers: []string{"le chat"}},
	}
	sc, err := s.Direct(ctx, Floor{Lang: french(), Depth: 1, Themes: []string{"Crypt"}, Words: entries,
		Monsters: []string{"Cave Bat"}, Quest: "Pets", QuestID: "a1", QuestWords: entries[:1]})
	if err != nil || sc.Name != "The Drowned Pantry" || sc.Names["Cave Bat"] != "Crumb Bat" {
		t.Fatalf("Direct: %+v, %v", sc, err)
	}
	dr := h.reqs[0].(gameai.DirectorRequest)
	if dr.Language != "fr" || dr.QuestID != "a1" || len(dr.Words) != 2 || len(dr.QuestWords) != 1 {
		t.Fatalf("director request %+v", dr)
	}
	if n, err := s.FillWords(ctx, french(), entries); err != nil || n != 2 {
		t.Fatalf("FillWords: %d, %v", n, err)
	}
	b := s.Bank("fr")
	if len(b.ClozeFor(entries[0])) != 1 || len(b.AllRiddles()["cat"]) != 1 || len(b.AllRiddles()["dog"]) != 0 {
		t.Fatalf("bank %v %v", b.AllCloze(), b.AllRiddles())
	}
	if n, err := s.FillTaunts(ctx, french(), entries); err != nil || n != 1 {
		t.Fatalf("FillTaunts: %d, %v", n, err)
	}
	if n, err := s.FillTips(ctx, french(), entries); err != nil || n != 1 {
		t.Fatalf("FillTips: %d, %v", n, err)
	}
	want := []string{"director", "cloze", "riddles", "taunts", "insight"}
	if len(h.asked) != len(want) {
		t.Fatalf("asked %v", h.asked)
	}
	for i := range want {
		if h.asked[i] != want[i] {
			t.Fatalf("asked %v", h.asked)
		}
	}
	// A spent allowance makes the game wait before asking again.
	h.err = ErrAllowance
	if _, err := s.FillTaunts(ctx, french(), entries); !errors.Is(err, ErrAllowance) {
		t.Fatal(err)
	}
	h.err = nil
	if _, err := s.FillTaunts(ctx, french(), entries); !errors.Is(err, ErrBusy) {
		t.Fatalf("asked again straight after the allowance ran out: %v", err)
	}
}
