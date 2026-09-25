package llm

import (
	"encoding/json"
	"math/rand/v2"
	"strings"
	"sync"

	"github.com/halpworld/halpwords/internal/words"
)

// Cloze is a sentence in the language being learned with a gap where a word
// goes.
type Cloze struct {
	Text    string // the sentence, with ___ for the word
	English string // the whole sentence in English
}

// Gap marks the missing word in a Cloze.
const Gap = "___"

// Taunt is a monster's battle cry in the language being learned.
type Taunt struct {
	Text    string
	English string
}

// Bank is the generated content for one language, kept between games so it
// is only paid for once. Words are identified by words.Key.
type Bank struct {
	mu      sync.Mutex
	lang    string
	Cloze   map[string][]Cloze  `json:",omitempty"`
	Riddles map[string][]string `json:",omitempty"` // by English word, lower case
	Taunts  []Taunt             `json:",omitempty"`
	Tips    map[string]string   `json:",omitempty"`
}

// Limits on how much the bank keeps.
const (
	perWord   = 3  // cloze sentences and riddles for each word
	maxTaunts = 40 // taunts for each language
)

func bankFile(lang string) string { return "ai/bank-" + lang + ".json" }

// Bank returns the generated content for language code lang.
func (s *Service) Bank(lang string) *Bank {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.banks == nil {
		s.banks = map[string]*Bank{}
	}
	if b := s.banks[lang]; b != nil {
		return b
	}
	b := &Bank{lang: lang}
	if s.store != nil {
		if data, err := s.store.Read(bankFile(lang)); err == nil {
			json.Unmarshal(data, b)
		}
	}
	s.banks[lang] = b
	return b
}

// saveBank writes b.
func (s *Service) saveBank(b *Bank) error {
	if s.store == nil {
		return nil
	}
	b.mu.Lock()
	data, err := json.Marshal(b)
	b.mu.Unlock()
	if err != nil {
		return err
	}
	return s.store.Write(bankFile(b.lang), data)
}

// ClozeFor returns the cloze sentences for e.
func (b *Bank) ClozeFor(e words.Entry) []Cloze {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]Cloze(nil), b.Cloze[words.Key(e)]...)
}

// AllCloze returns every cloze sentence, by words.Key.
func (b *Bank) AllCloze() map[string][]Cloze {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make(map[string][]Cloze, len(b.Cloze))
	for k, v := range b.Cloze {
		out[k] = append([]Cloze(nil), v...)
	}
	return out
}

// AllRiddles returns every generated riddle, by English word in lower case.
func (b *Bank) AllRiddles() map[string][]string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make(map[string][]string, len(b.Riddles))
	for k, v := range b.Riddles {
		out[k] = append([]string(nil), v...)
	}
	return out
}

// Taunt returns a random taunt, if the bank has any.
func (b *Bank) Taunt(rng *rand.Rand) (Taunt, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.Taunts) == 0 {
		return Taunt{}, false
	}
	return b.Taunts[rng.IntN(len(b.Taunts))], true
}

// TauntCount is how many taunts the bank has.
func (b *Bank) TauntCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.Taunts)
}

// Tip returns the memory tip for e, if there is one.
func (b *Bank) Tip(e words.Entry) (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	t, ok := b.Tips[words.Key(e)]
	return t, ok
}

// needsWords reports whether e has no cloze sentence or riddle yet.
func (b *Bank) needsWords(e words.Entry) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.Cloze[words.Key(e)]) == 0 && len(b.Riddles[strings.ToLower(e.Prompt)]) == 0
}

func (b *Bank) addCloze(e words.Entry, c Cloze) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.Cloze == nil {
		b.Cloze = map[string][]Cloze{}
	}
	k := words.Key(e)
	for _, o := range b.Cloze[k] {
		if o.Text == c.Text {
			return
		}
	}
	if len(b.Cloze[k]) < perWord {
		b.Cloze[k] = append(b.Cloze[k], c)
	}
}

func (b *Bank) addRiddle(e words.Entry, r string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.Riddles == nil {
		b.Riddles = map[string][]string{}
	}
	k := strings.ToLower(e.Prompt)
	for _, o := range b.Riddles[k] {
		if o == r {
			return
		}
	}
	if len(b.Riddles[k]) < perWord {
		b.Riddles[k] = append(b.Riddles[k], r)
	}
}

func (b *Bank) addTaunt(t Taunt) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, o := range b.Taunts {
		if o.Text == t.Text {
			return
		}
	}
	if len(b.Taunts) >= maxTaunts {
		b.Taunts = b.Taunts[1:]
	}
	b.Taunts = append(b.Taunts, t)
}

func (b *Bank) setTip(e words.Entry, tip string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.Tips == nil {
		b.Tips = map[string]string{}
	}
	b.Tips[words.Key(e)] = tip
}
