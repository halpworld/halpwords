package llm

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/halpworld/halpwords/pkg/safety"
	"github.com/halpworld/halpwords/pkg/words"
)

// Batch sizes: how many words one request covers.
const (
	wordBatch = 8
	tipBatch  = 4
	tauntMake = 8
)

// Lines of a prompt listing words, numbered from 1.
func wordLines(entries []words.Entry) string {
	var b strings.Builder
	for i, e := range entries {
		fmt.Fprintf(&b, "%d. %s = %s\n", i+1, e.Prompt, e.Answers[0])
	}
	return b.String()
}

// langLine names the language for a prompt, with a note on how it is
// written.
func langLine(lang *words.Language) string {
	switch lang.Code {
	case "grc":
		return "Ancient Greek (Attic, polytonic, with accents and breathings)"
	case "la":
		return "Latin (classical, with macrons where the word list uses them)"
	case "ga":
		return "Irish (Gaeilge, standard spelling with fadas)"
	}
	return lang.Name
}

// FillWords asks for cloze sentences and riddles for the first few entries
// that have neither yet, and keeps the ones that pass the checks. It
// returns how many words got something.
func (s *Service) FillWords(ctx context.Context, lang *words.Language, entries []words.Entry) (int, error) {
	bank := s.Bank(lang.Code)
	var todo []words.Entry
	for _, e := range entries {
		if len(e.Answers) > 0 && bank.needsWords(e) {
			todo = append(todo, e)
			if len(todo) == wordBatch {
				break
			}
		}
	}
	if len(todo) == 0 {
		return 0, nil
	}
	prompt := fmt.Sprintf(`Language being learned: %s.
For each numbered word below (English = %s answer), write:
- "cloze": one short, simple %s sentence (at most 60 characters) that uses the answer exactly as written, including any article, with the answer replaced by ___ (three underscores). The gap must not start the sentence, and the answer must be the natural word for the gap.
- "cloze_en": the whole sentence in English.
- "riddle": a playful English riddle of at most 90 characters, in the voice of a friendly dungeon, that describes the English word without using it.

Words:
%s
Reply with JSON only: {"items":[{"n":1,"cloze":"...","cloze_en":"...","riddle":"..."}]}`,
		langLine(lang), lang.Name, lang.Name, wordLines(todo))
	text, err := s.Ask(ctx, false, safety.Policy, prompt, 250*len(todo)+200)
	if err != nil {
		return 0, err
	}
	var out struct {
		Items []struct {
			N       int
			Cloze   string
			ClozeEn string `json:"cloze_en"`
			Riddle  string
		}
	}
	if err := decodeJSON(text, &out); err != nil {
		return 0, err
	}
	got := map[int]bool{}
	for _, it := range out.Items {
		if it.N < 1 || it.N > len(todo) {
			continue
		}
		e := todo[it.N-1]
		if c, ok := CheckCloze(it.Cloze, it.ClozeEn, e, lang); ok {
			bank.addCloze(e, c)
			got[it.N] = true
		}
		if r, ok := CheckRiddle(it.Riddle, e); ok {
			bank.addRiddle(e, r)
			got[it.N] = true
		}
	}
	return len(got), s.saveBank(bank)
}

var gaps = regexp.MustCompile(`_{2,}`)

// hasScript reports whether s has a letter in lang's script.
func hasScript(s string, lang *words.Language) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			continue
		}
		greek := unicode.Is(unicode.Greek, r)
		if greek == (lang.Script == words.ScriptGreek) {
			return true
		}
	}
	return false
}

// core is a word's answer without its article.
func core(answer string, lang *words.Language) string {
	low := strings.ToLower(answer)
	for _, a := range lang.Articles {
		if rest, ok := strings.CutPrefix(low, a); ok && rest != "" {
			return rest
		}
	}
	return low
}

// CheckCloze checks a generated cloze sentence for e and tidies it. The gap
// must stand for the whole first answer of e, so the local grader can mark
// it; the sentence must not give the answer away, and must be short enough
// to show.
func CheckCloze(text, english string, e words.Entry, lang *words.Language) (Cloze, bool) {
	text, english = tidy(text), tidy(english)
	text = gaps.ReplaceAllString(text, Gap)
	if strings.Count(text, Gap) != 1 || !short(text, 70) || !short(english, 100) {
		return Cloze{}, false
	}
	if !safety.Clean(text) || !safety.Clean(english) || !hasScript(text, lang) {
		return Cloze{}, false
	}
	answer := e.Answers[0]
	// "Le ___ aboie" for "le chien": the article is outside the gap. Take
	// it out so the gap is the whole answer.
	before, after, _ := strings.Cut(text, Gap)
	lowBefore := strings.ToLower(before)
	for _, a := range lang.Articles {
		if strings.HasPrefix(strings.ToLower(answer), a) && strings.HasSuffix(lowBefore, a) {
			before = before[:len(before)-len(a)]
			break
		}
	}
	text = before + Gap + after
	if c := core(answer, lang); len([]rune(c)) >= 3 && strings.Contains(strings.ToLower(before+after), c) {
		return Cloze{}, false // the answer is in the sentence already
	}
	if strings.EqualFold(strings.ReplaceAll(text, Gap, answer), english) {
		return Cloze{}, false // not translated
	}
	return Cloze{Text: text, English: english}, true
}

// CheckRiddle checks a generated riddle for e and tidies it.
func CheckRiddle(riddle string, e words.Entry) (string, bool) {
	riddle = tidy(riddle)
	if !short(riddle, 110) || !safety.Clean(riddle) {
		return "", false
	}
	low := strings.ToLower(riddle)
	for _, w := range strings.Fields(strings.ToLower(e.Prompt)) {
		if len(w) >= 3 && w != "the" && w != "and" && strings.Contains(low, w) {
			return "", false // gives the word away
		}
	}
	if strings.Contains(low, strings.ToLower(e.Answers[0])) {
		return "", false
	}
	return riddle, true
}

// FillTaunts asks for monster battle cries in lang, using words from
// entries where it can, and keeps the ones that pass the checks.
func (s *Service) FillTaunts(ctx context.Context, lang *words.Language, entries []words.Entry) (int, error) {
	sample := entries[:min(len(entries), 20)]
	prompt := fmt.Sprintf(`Language being learned: %s.
Write %d short battle cries that silly dungeon monsters shout at a young hero, in simple %s (at most 45 characters each), each with its English meaning. Playful boasting only, nothing cruel. Use some of these words where they fit:
%s
Reply with JSON only: {"taunts":[{"text":"...","english":"..."}]}`,
		langLine(lang), tauntMake, lang.Name, wordLines(sample))
	text, err := s.Ask(ctx, false, safety.Policy, prompt, 900)
	if err != nil {
		return 0, err
	}
	var out struct {
		Taunts []struct{ Text, English string }
	}
	if err := decodeJSON(text, &out); err != nil {
		return 0, err
	}
	bank, n := s.Bank(lang.Code), 0
	for _, t := range out.Taunts {
		if tt, ok := CheckTaunt(t.Text, t.English, lang); ok {
			bank.addTaunt(tt)
			n++
		}
	}
	return n, s.saveBank(bank)
}

// CheckTaunt checks a generated taunt and tidies it.
func CheckTaunt(text, english string, lang *words.Language) (Taunt, bool) {
	text, english = tidy(text), tidy(english)
	text = strings.Trim(text, "\"«»“”")
	english = strings.Trim(english, "\"“”()")
	if !short(text, 50) || !short(english, 70) || !safety.Clean(text) || !safety.Clean(english) ||
		!hasScript(text, lang) || strings.EqualFold(text, english) {
		return Taunt{}, false
	}
	return Taunt{Text: text, English: english}, true
}

// FillTips asks for memory tips for the first few entries that have none,
// for the Scroll of Insight at campfires.
func (s *Service) FillTips(ctx context.Context, lang *words.Language, entries []words.Entry) (int, error) {
	bank := s.Bank(lang.Code)
	var todo []words.Entry
	for _, e := range entries {
		if _, ok := bank.Tip(e); !ok && len(e.Answers) > 0 {
			todo = append(todo, e)
			if len(todo) == tipBatch {
				break
			}
		}
	}
	if len(todo) == 0 {
		return 0, nil
	}
	prompt := fmt.Sprintf(`Language being learned: %s.
A student keeps misspelling these words. For each, write one memory tip of at most 100 characters: a vivid memory hook, a link to an English word, or a note on the tricky letters. Be accurate; only give an etymology you are sure of.
%s
Reply with JSON only: {"tips":[{"n":1,"tip":"..."}]}`, langLine(lang), wordLines(todo))
	text, err := s.Ask(ctx, false, safety.Policy, prompt, 120*len(todo)+120)
	if err != nil {
		return 0, err
	}
	var out struct {
		Tips []struct {
			N   int
			Tip string
		}
	}
	if err := decodeJSON(text, &out); err != nil {
		return 0, err
	}
	n := 0
	for _, t := range out.Tips {
		tip := tidy(t.Tip)
		if t.N < 1 || t.N > len(todo) || !short(tip, 120) || !safety.Clean(tip) {
			continue
		}
		bank.setTip(todo[t.N-1], tip)
		n++
	}
	return n, s.saveBank(bank)
}
