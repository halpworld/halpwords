package link

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/halpworld/halpwords/pkg/words"
)

// syncMemory downloads the learner's word memory in each language of the
// assigned lists, for the game loop to merge with MergeMemory. syncMu is
// held.
func (c *Client) syncMemory(ctx context.Context, gen int) error {
	c.mu.Lock()
	var langs []string
	for _, li := range c.st.Lists {
		if !slices.Contains(langs, li.Language) {
			langs = append(langs, li.Language)
		}
	}
	uploads := c.uploads
	c.mu.Unlock()
	for _, lang := range langs {
		var body struct {
			Lang   string        `json:"lang"`
			Memory *words.Memory `json:"memory"`
		}
		if _, _, err := c.authed(ctx, gen, http.MethodGet, "/api/v1/memory?lang="+url.QueryEscape(lang), nil, nil, &body); err != nil {
			return err
		}
		if body.Memory == nil {
			continue
		}
		c.mu.Lock()
		if c.gen != gen {
			c.mu.Unlock()
			return ErrNotLinked
		}
		c.memories[lang] = &fetched{mem: body.Memory, uploads: uploads}
		c.changes++
		c.mu.Unlock()
	}
	return nil
}

// MergeMemory merges the word memories the server sent into the player's
// memories (by language code), and reports whether any changed. The game
// loop calls it when Changes moves on; it doesn't wait for the network.
//
// Words the server has cards for take the server's cards, which hold
// every answer from all the learner's games. Answers still waiting in the
// queue are then replayed on top, as the server hasn't seen them. Other
// words (the player's own lists) keep their local cards, and the local
// Clock stays: the server's due times are moved to it.
func (c *Client) MergeMemory(mems map[string]*words.Memory) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	changed := false
	for lang, f := range c.memories {
		delete(c.memories, lang)
		if f.uploads != c.uploads || !c.st.linked() {
			// Answers were sent since it was made: the next sync fetches
			// a memory that has them.
			continue
		}
		local := mems[lang]
		if local == nil {
			local = words.NewMemory()
			mems[lang] = local
		}
		merge(local, f.mem, c.q.Answers, lang)
		changed = true
	}
	return changed
}

// merge puts the server's cards into local and replays the queued answers
// to those words on top.
func merge(local, server *words.Memory, queued []qAnswer, lang string) {
	var replay []qAnswer
	for _, a := range queued {
		if a.Lang == lang && server.Cards[a.Word] != nil {
			replay = append(replay, a)
		}
	}
	// The queued answers are already counted in the local Clock; the
	// server's cards are moved to where it stood before them.
	base := max(0, local.Clock-len(replay))
	tmp := &words.Memory{Clock: base, Cards: map[string]*words.Card{}}
	for k, sc := range server.Cards {
		if sc == nil {
			continue
		}
		card := *sc
		card.Due += base - server.Clock
		tmp.Cards[k] = &card
	}
	for _, a := range replay {
		ans := words.Answer{Tier: words.Tier(a.Tier), Hinted: a.Hinted}
		if a.Secs != nil {
			ans.Timed, ans.Secs = true, *a.Secs
		}
		if a.Mistake != nil {
			ans.Mistake = words.Mistake(*a.Mistake)
		}
		tmp.Record(entryFor(a.Word), ans)
	}
	if local.Cards == nil {
		local.Cards = map[string]*words.Card{}
	}
	for k, card := range tmp.Cards {
		local.Cards[k] = card
	}
	local.Clock = max(local.Clock, tmp.Clock)
}

// entryFor turns a words.Key back into an entry with the same key: its
// prompt and first answer. A prompt never holds "=", so the first " = "
// divides them.
func entryFor(key string) words.Entry {
	p, a, _ := strings.Cut(key, " = ")
	return words.Entry{Prompt: p, Answers: []string{a}}
}
