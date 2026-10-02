package compete

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"github.com/halpworld/halpwords/pkg/words"
)

// ListHash names a set of word lists by their words, so runs are only
// compared with runs over the same words (halpwords-server's rankings):
// 16 hex digits. The order of the lists and words doesn't matter.
func ListHash(entries []words.Entry) string {
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, words.Key(e))
	}
	sort.Strings(keys)
	h := sha256.New()
	h.Write([]byte("halpwords-lists"))
	for _, k := range keys {
		h.Write([]byte{0})
		h.Write([]byte(k))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// Run is a finished scored run as a linked game sends it to be ranked:
// its share code's contents, what it counted and how long it was played.
type Run struct {
	Share Share
	Tally Tally
	// Secs is how long the run was played, in seconds, across suspends.
	Secs int
}

// Limits of what a run can hold, far beyond what anyone plays, so a
// changed number can't overflow a sum.
const (
	MaxFloor = 10_000
	MaxSecs  = 7 * 24 * 60 * 60
)

// Plausibility: the least time the game takes for a floor (walking to
// the stairs, at least six cells at 150 ms) and for a typed word (the
// word appears, and the answer is typed and checked).
const (
	MinFloorSecs = 0.9
	MinWordSecs  = 0.5
)

// ErrImplausible is a run the game can't have played: its score isn't
// its tally's, or it did more than there was time or room for.
var ErrImplausible = errors.New("that run doesn't add up")

// Check reports whether a run is one the game can have played: the
// share code's score is the tally's, and the tally fits the floors and
// the time. It is not proof against cheating, which needs the run to be
// replayed (verified runs), but it refuses a changed score or tally.
func (r Run) Check() error {
	s, t := r.Share, r.Tally
	bad := func(why string) error { return fmt.Errorf("%w: %s", ErrImplausible, why) }
	switch {
	case s.Floor < 1 || s.Floor > MaxFloor:
		return bad("floor out of range")
	case r.Secs < 0 || r.Secs > MaxSecs:
		return bad("time out of range")
	case t.Damage < 0 || t.Perfect < 0 || t.BestCombo < 0 || t.Bosses < 0 || t.Chests < 0 || t.Misses < 0:
		return bad("a negative count")
	case t.Score(s.Floor) != s.Score:
		return bad("the score isn't the tally's")
	case float64(r.Secs) < float64(s.Floor-1)*MinFloorSecs:
		return bad("floors faster than the stairs can be reached")
	case float64(t.Perfect+t.Misses) > float64(r.Secs)/MinWordSecs+1,
		float64(t.BestCombo) > float64(r.Secs)/MinWordSecs+1:
		return bad("more words than there was time to type")
	case t.Bosses > s.Floor:
		return bad("more bosses than floors")
	case t.Chests > MaxChests*s.Floor:
		return bad("more chests than the floors hold")
	case t.Damage > MaxDamage(s.Floor):
		return bad("more damage than the floors' monsters have")
	}
	return nil
}

// MaxChests is more than the most chests a floor has (two or three, and
// a mimic is one of them).
const MaxChests = 4

// MaxDamage is more than all the damage the monsters on floors 1 to floor
// can take: the most monsters a floor has (those placed on it, a boss
// and a mimic in every chest), each with the most hit points of any kind
// at that depth, twice over for monsters that heal. Damage counts no more
// than a monster's hit points.
func MaxDamage(floor int) int {
	const maxHP = 80 // the strongest boss has 70 at depth 1
	total := 0
	for d := 1; d <= floor; d++ {
		monsters := 3 + d + 1 + MaxChests
		total += monsters * (maxHP*(100+15*(d-1))/100 + 1)
	}
	return 2 * total
}
