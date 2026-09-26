package llm

import (
	"context"
	"errors"

	"github.com/halpworld/halpwords/pkg/gameai"
	"github.com/halpworld/halpwords/pkg/words"
)

// Halpwords is Halpwords AI: the server's AI, for a game linked to a
// grown-up's account whose plan includes it. It needs no key and has no
// budget here: the server keeps to the account's allowance. It writes the
// same things as a model with the player's own key (floor scripts, gap-fill
// sentences, riddles, taunts and memory tips) but not word lists.
const Halpwords ProviderID = "halpwords"

// HalpwordsAI is how the service reaches Halpwords AI: the game's link to
// the server.
type HalpwordsAI interface {
	// Available reports whether the game is linked and the account's plan
	// includes Halpwords AI. It must not wait for the network.
	Available() bool
	// Do sends req to POST /api/v1/ai/{task} and reads the answer into
	// out. Its errors are the service's: ErrBusy when the server asks the
	// game to wait, ErrAllowance when the month's allowance is spent,
	// ErrNoHalpwords when the plan does not include it (any more), and
	// ErrEmpty when the server had nothing it could use.
	Do(ctx context.Context, task string, req, out any) error
}

// Errors of Halpwords AI.
var (
	// ErrNoHalpwords is Halpwords AI chosen for a game that isn't linked,
	// or whose account's plan doesn't include it.
	ErrNoHalpwords = errors.New("the game needs linking to an account whose plan includes Halpwords AI")
	// ErrAllowance is Halpwords AI's allowance for the month, used up.
	ErrAllowance = errors.New("this month's Halpwords AI is used up")
	// ErrNeedsKey is a request only a model with the player's own key
	// can answer, such as the Word Forge's.
	ErrNeedsKey = errors.New("this needs an AI with its own API key")
)

// UseHalpwords connects the service to Halpwords AI. With nil, it isn't
// used.
func (s *Service) UseHalpwords(h HalpwordsAI) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hw = h
}

// HalpwordsAvailable reports whether Halpwords AI can be used now: the
// game is linked and the account's plan includes it.
func (s *Service) HalpwordsAvailable() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hwAvailable()
}

func (s *Service) hwAvailable() bool { return s.hw != nil && s.hw.Available() }

// wantsHalpwords reports whether the settings choose Halpwords AI: chosen
// in the AI Helper screen, or no AI chosen and Halpwords AI not turned
// off, so a linked game uses what its grown-up's plan includes without
// setting anything up. The lock must be held.
func (s *Service) wantsHalpwords() bool {
	return s.cfg.Provider == Halpwords || (s.cfg.Provider == Off && !s.cfg.NoHalpwords)
}

// Chosen is what the AI Helper screen shows as chosen: Halpwords when
// Halpwords AI is chosen or used, a provider's ID, or Off.
func (s *Service) Chosen() ProviderID {
	if s == nil {
		return Off
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case s.cfg.Provider == Halpwords, s.wantsHalpwords() && s.hwAvailable():
		return Halpwords
	case ProviderFor(s.cfg.Provider) != nil:
		return s.cfg.Provider
	}
	return Off
}

// UsingHalpwords reports whether the game's AI is Halpwords AI now.
func (s *Service) UsingHalpwords() bool { return s.halpwordsAI() != nil }

// halpwordsAI returns Halpwords AI when the game's AI is Halpwords AI now,
// or nil.
func (s *Service) halpwordsAI() HalpwordsAI {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.wantsHalpwords() && s.hwAvailable() {
		return s.hw
	}
	return nil
}

// ForgeReady reports whether the Word Forge can be used: an AI with the
// player's own key is Ready. Halpwords AI doesn't make word lists.
func (s *Service) ForgeReady() bool { return s.Ready() && !s.UsingHalpwords() }

// askHalpwords sends a request to Halpwords AI, one of maxRunning at a
// time, and waits after the server says it is busy.
func (s *Service) askHalpwords(ctx context.Context, h HalpwordsAI, task string, req, out any) error {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		return ctx.Err()
	}
	s.mu.Lock()
	now := s.now()
	if now.Before(s.paused) {
		s.mu.Unlock()
		return ErrBusy
	}
	s.mu.Unlock()
	err := h.Do(ctx, task, req, out)
	if errors.Is(err, ErrBusy) || errors.Is(err, ErrAllowance) {
		s.mu.Lock()
		s.paused = s.now().Add(busyPause)
		s.mu.Unlock()
	}
	return err
}

// directorRequest is f as a request to Halpwords AI.
func directorRequest(f Floor) gameai.DirectorRequest {
	r := gameai.DirectorRequest{
		Language:   f.Lang.Code,
		Depth:      f.Depth,
		Themes:     f.Themes[:min(len(f.Themes), gameai.MaxThemes)],
		Monsters:   f.Monsters[:min(len(f.Monsters), gameai.MaxMonsters)],
		Boss:       f.Boss,
		Words:      gameai.WordsOf(f.Words[:min(len(f.Words), gameai.DirectorWords)]),
		QuestID:    f.QuestID,
		QuestWords: gameai.WordsOf(f.QuestWords[:min(len(f.QuestWords), gameai.QuestWords)]),
	}
	if f.QuestID == "" {
		r.QuestWords = nil // the server only builds floors around quests it knows
	}
	return r
}

// wordsRequest is a request to Halpwords AI about entries.
func wordsRequest(lang *words.Language, entries []words.Entry) gameai.WordsRequest {
	return gameai.WordsRequest{Language: lang.Code, Words: gameai.WordsOf(entries)}
}
