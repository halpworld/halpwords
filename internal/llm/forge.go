package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/halpworld/halpwords/pkg/safety"
	"github.com/halpworld/halpwords/pkg/words"
)

// ForgeCounts are the list sizes the Word Forge offers.
var ForgeCounts = []int{10, 20, 30}

// ErrTopic is returned for a Word Forge topic the game won't send.
var ErrTopic = errors.New("please choose another topic")

// Forge makes a new word list of about count words on topic in lang, with
// the word list model. A second request checks the translations: words it
// finds wrong are fixed or dropped. The list still needs saving.
func (s *Service) Forge(ctx context.Context, lang *words.Language, topic string, count int) (*words.List, error) {
	topic = tidy(topic)
	if !short(topic, 60) || !safety.Clean(topic) {
		return nil, ErrTopic
	}
	prompt := fmt.Sprintf(`Make a vocabulary list for a 13-year-old learning %s.
Topic: %s
Give %d useful words or short phrases on the topic, easiest first, in two to four groups.
- "en": the English word, lower case, at most 30 characters.
- "answer": the %s, at most 30 characters, spelled exactly as a textbook would, with all accents. Give nouns with their article where the language has one (French: le/la/l'/les; Irish nouns without an article; Latin and Greek nouns in the nominative, Greek with its article).
- "alt": other correct answers, if any (such as another article form), else [].
Reply with JSON only:
{"title": "a short list title, at most 24 characters", "groups": [{"tag": "group name, one or two words", "words": [{"en": "...", "answer": "...", "alt": []}]}]}`,
		langLine(lang), topic, count, lang.Name)
	text, err := s.Ask(ctx, true, safety.Policy, prompt, 60*count+400)
	if err != nil {
		return nil, err
	}
	var out struct {
		Title  string
		Groups []struct {
			Tag   string
			Words []struct {
				En     string
				Answer string
				Alt    []string
			}
		}
	}
	if err := decodeJSON(text, &out); err != nil {
		return nil, err
	}
	title := tidy(out.Title)
	if !short(title, 30) || !safety.Clean(title) || !nameLike(title) {
		title = topic
	}
	l := &words.List{Title: lang.Name + " - " + title + " (forged)", Language: lang.Code}
	seen := map[string]bool{}
	for _, g := range out.Groups {
		tag := strings.ToLower(tidy(g.Tag))
		if !short(tag, 20) || !safety.Clean(tag) || !nameLike(tag) {
			tag = ""
		}
		for _, w := range g.Words {
			e, ok := checkForged(w.En, w.Answer, w.Alt, lang)
			if !ok || seen[strings.ToLower(e.Prompt)] {
				continue
			}
			seen[strings.ToLower(e.Prompt)] = true
			e.Tag = tag
			l.Entries = append(l.Entries, e)
		}
	}
	if len(l.Entries) < 3 {
		return nil, ErrEmpty
	}
	l.File = words.FileName(l.Title)
	if err := s.proofread(ctx, lang, l); err != nil {
		return nil, fmt.Errorf("checking the words: %w", err)
	}
	if len(l.Entries) < 3 {
		return nil, ErrEmpty
	}
	return l, nil
}

// checkForged checks one generated word.
func checkForged(en, answer string, alt []string, lang *words.Language) (words.Entry, bool) {
	ok := func(s string, script *words.Language) bool {
		return short(s, 30) && safety.Clean(s) && !strings.ContainsAny(s, "=|#") && (script == nil || hasScript(s, script))
	}
	en, answer = strings.ToLower(tidy(en)), tidy(answer)
	if !ok(en, words.English) || !ok(answer, lang) {
		return words.Entry{}, false
	}
	e := words.Entry{Prompt: en, Answers: []string{answer}}
	for _, a := range alt {
		if a = tidy(a); ok(a, lang) && !strings.EqualFold(a, answer) && len(e.Answers) < 4 {
			e.Answers = append(e.Answers, a)
		}
	}
	return e, true
}

// proofread asks the model to check each translation in l, and fixes or
// drops the words it finds wrong.
func (s *Service) proofread(ctx context.Context, lang *words.Language, l *words.List) error {
	prompt := fmt.Sprintf(`You are a careful %s teacher. Check this vocabulary list (English = %s) for a 13-year-old.
For each numbered line, say whether the %s is a correct, standard translation, spelled correctly with all accents and the right article.
%s
Reply with JSON only: {"checks":[{"n":1,"ok":true,"fix":""}]}
When a line is wrong, set "ok" to false and put the corrected %s in "fix", or leave "fix" empty if the line should be removed.`,
		langLine(lang), lang.Name, lang.Name, wordLines(l.Entries), lang.Name)
	text, err := s.Ask(ctx, true, safety.Policy, prompt, 30*len(l.Entries)+300)
	if err != nil {
		return err
	}
	var out struct {
		Checks []struct {
			N   int
			OK  bool
			Fix string
		}
	}
	if err := decodeJSON(text, &out); err != nil {
		return err
	}
	drop := map[int]bool{}
	for _, c := range out.Checks {
		if c.N < 1 || c.N > len(l.Entries) || c.OK {
			continue
		}
		i := c.N - 1
		if fixed, ok := checkForged(l.Entries[i].Prompt, c.Fix, nil, lang); ok && c.Fix != "" {
			l.Entries[i].Answers = fixed.Answers // alternatives may be wrong too
		} else {
			drop[i] = true
		}
	}
	kept := l.Entries[:0]
	for i, e := range l.Entries {
		if !drop[i] {
			kept = append(kept, e)
		}
	}
	l.Entries = kept
	return nil
}
