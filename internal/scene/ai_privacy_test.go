package scene

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/halpworld/halpwords/internal/rpg"
	"github.com/halpworld/halpwords/pkg/words"
)

// ownWords is a child's own list, typed in the game: none of it may reach
// an AI. "dog" is also a starter word; the extra answer is the child's.
var ownWords = []words.Entry{
	{Prompt: "zebrafish", Answers: []string{"le poisson-zèbre"}},
	{Prompt: "axolotl", Answers: []string{"l'axolotl"}},
	{Prompt: "my cat Biscuit", Answers: []string{"Biscuit"}},
	{Prompt: "dog", Answers: []string{"le chien", "le toutou"}},
}

// ownSecrets are bits of ownWords that are in no starter list.
var ownSecrets = []string{"zebrafish", "axolotl", "Biscuit", "toutou", "poisson-z"}

func secretList() *words.List {
	return &words.List{Title: "My secret words", File: "secret.txt", Language: "fr", Entries: ownWords}
}

// An own list's words never go to an AI, in a run that mixes it with a
// starter list: not to the Director, the gap-fills and riddles, the
// taunts or the memory tips.
func TestOwnWordsNeverReachTheAI(t *testing.T) {
	ctx, f := aiContext(t)
	ctx.Lists = append(ctx.Lists, secretList())
	lang, _ := words.Lookup("fr")
	r := beginRun(ctx, lang, rpg.Knight, 11, nil, nil, listPool{keys: []string{"file:french.txt", "file:secret.txt"}})
	c := newCrawl(r)
	// Miss every own word twice, and a starter word, so they lead the
	// floor's words and each asks for a memory tip.
	var own []int // the own words in no starter list
	for id, e := range r.deck.Entries() {
		mine := false
		for _, o := range ownWords[:3] {
			mine = mine || words.Key(o) == words.Key(e)
		}
		if mine {
			own = append(own, id)
		}
		if mine || e.Prompt == "cat" || slices.Contains(e.Answers, "le toutou") {
			for range 2 {
				c.scoreAnswer(id, words.Result{Tier: words.Miss, Expected: e.Answers[0]}, "x", false, 0)
			}
		}
	}
	if len(own) != 3 {
		t.Fatalf("own words in the deck: %d", len(own))
	}
	pump(t, c, "every kind of request", func() bool {
		return f.count("director") > 0 && f.count("words") > 0 && f.count("taunts") > 0 && f.count("tips") > 0 &&
			r.ai.work == nil && len(r.ai.tips) == 0
	})
	f.mu.Lock()
	sent := strings.Join(f.sent, "\n")
	f.mu.Unlock()
	for _, s := range ownSecrets {
		if strings.Contains(sent, s) {
			t.Errorf("an AI was sent %q from the child's own list", s)
		}
	}
	// The starter words did go, so the test is not passing on nothing.
	if !strings.Contains(sent, "cat = le chat") {
		t.Error("no starter word was sent")
	}
	for _, id := range own {
		if _, ok := r.ai.tipFor(r, id); ok {
			t.Errorf("a memory tip for own word %q", r.deck.Entries()[id].Prompt)
		}
	}
	if r.ai.fails != 0 {
		t.Errorf("%d failed requests", r.ai.fails)
	}
}

// A run of own words only asks the AI nothing, and plays on quietly with
// the offline floors and puzzles.
func TestOwnOnlyRunAsksNothing(t *testing.T) {
	ctx, f := aiContext(t)
	ctx.Lists = append(ctx.Lists, secretList())
	lang, _ := words.Lookup("fr")
	r := beginRun(ctx, lang, rpg.Knight, 11, nil, nil, listPool{keys: []string{"file:secret.txt"}})
	if len(r.deck.Entries()) != len(ownWords) {
		t.Fatalf("deck of %d", len(r.deck.Entries()))
	}
	c := newCrawl(r)
	e, id := r.deck.Next()
	for range 2 {
		c.scoreAnswer(id, words.Result{Tier: words.Miss, Expected: e.Answers[0]}, "x", false, 0)
	}
	logs := len(r.log)
	for range 200 {
		r.ai.poll(c)
		time.Sleep(time.Millisecond)
	}
	for _, kind := range []string{"director", "words", "taunts", "tips"} {
		if n := f.count(kind); n != 0 {
			t.Errorf("asked for %s %d times", kind, n)
		}
	}
	if r.ai.fails != 0 || !r.ai.on() {
		t.Errorf("the AI counted %d failures", r.ai.fails)
	}
	for _, l := range r.log[logs:] {
		if strings.Contains(l.text, "AI") || strings.Contains(l.text, "dungeon stirs") {
			t.Errorf("log: %q", l.text)
		}
	}
	if r.script() != nil {
		t.Error("a floor script from nowhere")
	}
}
