package llm

import "github.com/halpworld/halpwords/pkg/words"

// The words of a child's own lists, typed in the game or imported, never
// leave the device (#89). Every request that carries words takes them
// through shareable, whatever the backend: the player's own key,
// Halpwords AI, or one added later. Only the words the game said may be
// shared go: the built-in lists and the lists a teacher or parent sent or
// assigned. Until the game says, no word goes.

// ShareOnly sets the words that may be sent to an AI: every other word is
// left out of requests. The entry sent is the one given here, so a word
// that is also in a child's own list goes without what they added to it
// (other answers, a group tag).
func (s *Service) ShareOnly(entries []words.Entry) {
	ok := make(map[string]words.Entry, len(entries))
	for _, e := range entries {
		if _, dup := ok[words.Key(e)]; !dup {
			ok[words.Key(e)] = e
		}
	}
	s.mu.Lock()
	s.share = ok
	s.mu.Unlock()
}

// shareable returns the entries that may be sent to an AI, as ShareOnly
// gave them, in order.
func (s *Service) shareable(entries []words.Entry) []words.Entry {
	s.mu.Lock()
	ok := s.share
	s.mu.Unlock()
	var out []words.Entry
	for _, e := range entries {
		if e, found := ok[words.Key(e)]; found {
			out = append(out, e)
		}
	}
	return out
}
