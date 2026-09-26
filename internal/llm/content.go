package llm

import (
	"context"

	"github.com/halpworld/halpwords/pkg/gameai"
	"github.com/halpworld/halpwords/pkg/safety"
	"github.com/halpworld/halpwords/pkg/words"
)

// FillWords asks for cloze sentences and riddles for the first few entries
// that have neither yet, and keeps the ones that pass the checks. It
// returns how many words got something.
func (s *Service) FillWords(ctx context.Context, lang *words.Language, entries []words.Entry) (int, error) {
	bank := s.Bank(lang.Code)
	var todo []words.Entry
	for _, e := range entries {
		if len(e.Answers) > 0 && bank.needsWords(e) {
			todo = append(todo, e)
			if len(todo) == gameai.WordBatch {
				break
			}
		}
	}
	if len(todo) == 0 {
		return 0, nil
	}
	var cloze, riddles gameai.WordsReply
	if h := s.halpwordsAI(); h != nil {
		// Halpwords AI asks for each separately, so each is kept (and
		// paid for) on its own.
		req := wordsRequest(lang, todo)
		if err := s.askHalpwords(ctx, h, gameai.Cloze, req, &cloze); err != nil {
			return 0, err
		}
		if err := s.askHalpwords(ctx, h, gameai.Riddles, req, &riddles); err != nil {
			n := s.keepWords(bank, lang, todo, cloze, riddles)
			s.saveBank(bank)
			return n, err
		}
	} else {
		text, err := s.Ask(ctx, false, safety.Policy, gameai.WordsPrompt(lang, todo), gameai.WordsTokens(len(todo)))
		if err != nil {
			return 0, err
		}
		if err := decodeJSON(text, &cloze); err != nil {
			return 0, err
		}
		riddles = cloze
	}
	return s.keepWords(bank, lang, todo, cloze, riddles), s.saveBank(bank)
}

// keepWords puts the sentences and riddles that pass the checks in the
// bank, and returns how many words got something.
func (s *Service) keepWords(bank *Bank, lang *words.Language, todo []words.Entry, cloze, riddles gameai.WordsReply) int {
	got := map[int]bool{}
	for i, c := range gameai.KeepCloze(cloze, lang, todo) {
		bank.addCloze(todo[i], c)
		got[i] = true
	}
	for i, r := range gameai.KeepRiddles(riddles, todo) {
		bank.addRiddle(todo[i], r)
		got[i] = true
	}
	return len(got)
}

// CheckCloze checks a generated cloze sentence for e and tidies it. The gap
// must stand for the whole first answer of e, so the local grader can mark
// it; the sentence must not give the answer away, and must be short enough
// to show.
func CheckCloze(text, english string, e words.Entry, lang *words.Language) (Cloze, bool) {
	return gameai.CheckCloze(text, english, e, lang)
}

// CheckRiddle checks a generated riddle for e and tidies it.
func CheckRiddle(riddle string, e words.Entry) (string, bool) { return gameai.CheckRiddle(riddle, e) }

// CheckTaunt checks a generated taunt and tidies it.
func CheckTaunt(text, english string, lang *words.Language) (Taunt, bool) {
	return gameai.CheckTaunt(text, english, lang)
}

// FillTaunts asks for monster battle cries in lang, using words from
// entries where it can, and keeps the ones that pass the checks.
func (s *Service) FillTaunts(ctx context.Context, lang *words.Language, entries []words.Entry) (int, error) {
	var out gameai.TauntsReply
	if h := s.halpwordsAI(); h != nil {
		if err := s.askHalpwords(ctx, h, gameai.Taunts, wordsRequest(lang, entries[:min(len(entries), gameai.TauntWords)]), &out); err != nil {
			return 0, err
		}
	} else {
		text, err := s.Ask(ctx, false, safety.Policy, gameai.TauntsPrompt(lang, entries), gameai.TauntsTokens)
		if err != nil {
			return 0, err
		}
		if err := decodeJSON(text, &out); err != nil {
			return 0, err
		}
	}
	bank := s.Bank(lang.Code)
	kept := gameai.KeepTaunts(out, lang)
	for _, t := range kept {
		bank.addTaunt(t)
	}
	return len(kept), s.saveBank(bank)
}

// FillTips asks for memory tips for the first few entries that have none,
// for the Scroll of Insight at campfires.
func (s *Service) FillTips(ctx context.Context, lang *words.Language, entries []words.Entry) (int, error) {
	bank := s.Bank(lang.Code)
	var todo []words.Entry
	for _, e := range entries {
		if _, ok := bank.Tip(e); !ok && len(e.Answers) > 0 {
			todo = append(todo, e)
			if len(todo) == gameai.TipBatch {
				break
			}
		}
	}
	if len(todo) == 0 {
		return 0, nil
	}
	var out gameai.TipsReply
	if h := s.halpwordsAI(); h != nil {
		if err := s.askHalpwords(ctx, h, gameai.Insight, wordsRequest(lang, todo), &out); err != nil {
			return 0, err
		}
	} else {
		text, err := s.Ask(ctx, false, safety.Policy, gameai.TipsPrompt(lang, todo), gameai.TipsTokens(len(todo)))
		if err != nil {
			return 0, err
		}
		if err := decodeJSON(text, &out); err != nil {
			return 0, err
		}
	}
	kept := gameai.KeepTips(out, todo)
	for i, tip := range kept {
		bank.setTip(todo[i], tip)
	}
	return len(kept), s.saveBank(bank)
}
