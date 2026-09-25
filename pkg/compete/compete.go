// Package compete holds what Hardcore runs are compared by: the score, seed
// codes, the Daily Dungeon, share codes and the Hall of Fame. It has no
// Ebitengine dependency.
package compete

import (
	"errors"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/halpworld/halpwords/pkg/words"
)

// Mode is how an adventure is played.
type Mode uint8

const (
	// Adventure has Save Shrines: a fallen hero wakes at the last one.
	Adventure Mode = iota
	// Hardcore has one life, no shrines and a score.
	Hardcore
	// Daily is Hardcore in the day's dungeon, the same for everyone with
	// the same word lists.
	Daily
)

func (m Mode) String() string {
	switch m {
	case Hardcore:
		return "Hardcore"
	case Daily:
		return "Daily Dungeon"
	}
	return "Adventure"
}

// Scored reports whether runs in mode m have a score.
func (m Mode) Scored() bool { return m == Hardcore || m == Daily }

// Tally counts what a run has done, for its score.
type Tally struct {
	Damage    int // damage dealt to monsters
	Perfect   int // words spelled perfectly, without a hint
	BestCombo int // the longest streak of good answers
	Bosses    int
	Chests    int
	Misses    int
}

// Points for each part of the score.
const (
	FloorPoints   = 1000
	PerfectPoints = 50
	ComboPoints   = 100
	BossPoints    = 2500
	ChestPoints   = 150
	MissPoints    = 25
)

// Score is a run's score on reaching floor. It never goes below 0.
func (t Tally) Score(floor int) int {
	s := floor*FloorPoints + t.Damage + t.Perfect*PerfectPoints + t.BestCombo*ComboPoints +
		t.Bosses*BossPoints + t.Chests*ChestPoints - t.Misses*MissPoints
	return max(0, s)
}

// alphabet is Crockford's base 32: no I, L, O or U, so codes are easy to
// read out and type.
const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// SeedBits is the size of a seed that fits in a seed code.
const SeedBits = 30

// seedCodeLen is the length of a seed code.
const seedCodeLen = SeedBits / 5

// seedMask keeps the bits of a seed that a code holds.
const seedMask = 1<<SeedBits - 1

// RandomSeed returns a new dungeon seed that has a seed code.
func RandomSeed(rng *rand.Rand) uint64 { return rng.Uint64() & seedMask }

// SeedCode is the 6-character code for seed, such as "7K3QZP". Only the
// seed's low 30 bits are kept.
func SeedCode(seed uint64) string { return encode(seed&seedMask, seedCodeLen) }

func encode(v uint64, n int) string {
	b := make([]byte, n)
	for i := n - 1; i >= 0; i-- {
		b[i] = alphabet[v&31]
		v >>= 5
	}
	return string(b)
}

// clean upper-cases a typed code and drops spaces.
func clean(s string) string {
	return strings.ToUpper(strings.Join(strings.Fields(s), ""))
}

// fix reads letters that are easily mistaken for digits in a base-32 part
// of a code as the digits.
func fix(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case 'O':
			r = '0'
		case 'I', 'L':
			r = '1'
		}
		b.WriteRune(r)
	}
	return b.String()
}

func decode(s string) (uint64, bool) {
	var v uint64
	for _, r := range s {
		i := strings.IndexRune(alphabet, r)
		if i < 0 {
			return 0, false
		}
		v = v<<5 | uint64(i)
	}
	return v, true
}

// ErrBadCode is returned for a code that can't be read.
var ErrBadCode = errors.New("that code doesn't look right")

// ParseSeed reads a seed code. Capitals don't matter, and O, I and L are
// read as 0, 1 and 1.
func ParseSeed(code string) (uint64, error) {
	c := fix(clean(code))
	if len(c) != seedCodeLen {
		return 0, ErrBadCode
	}
	v, ok := decode(c)
	if !ok {
		return 0, ErrBadCode
	}
	return v, nil
}

// DailySeed is the Daily Dungeon's seed on day in language lang with the
// words entries. Everyone with the same word lists gets the same dungeon;
// the order of the lists and words doesn't matter.
func DailySeed(day time.Time, lang string, entries []words.Entry) uint64 {
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		keys = append(keys, words.Key(e))
	}
	sort.Strings(keys)
	h := fnv.New64a()
	fmt.Fprintf(h, "daily|%s|%s", day.Format(time.DateOnly), lang)
	for _, k := range keys {
		h.Write([]byte{0})
		h.Write([]byte(k))
	}
	return h.Sum64() & seedMask
}

// Share is what a share code holds about a finished run.
type Share struct {
	Lang  string // a language code, such as "fr"
	Daily bool
	// Month and Day are the date of a Daily Dungeon.
	Month time.Month
	Day   int
	// Seed is the dungeon of any other run.
	Seed  uint64
	Floor int
	Score int
}

// checkLen is the length of a share code's checksum.
const checkLen = 4

// Code returns the share code, such as "HW-FR-0922-F12-18450-K7QX" for a
// Daily Dungeon or "HW-FR-7K3QZP-F12-18450-M2B9" for a seed.
func (s Share) Code() string {
	body := s.body()
	return body + "-" + checksum(body)
}

func (s Share) body() string {
	run := SeedCode(s.Seed)
	if s.Daily {
		run = fmt.Sprintf("%02d%02d", int(s.Month), s.Day)
	}
	return fmt.Sprintf("HW-%s-%s-F%d-%d", strings.ToUpper(s.Lang), run, s.Floor, s.Score)
}

func checksum(body string) string {
	h := fnv.New32a()
	h.Write([]byte("halpwords|" + body))
	return encode(uint64(h.Sum32()), checkLen)
}

// ErrBadChecksum is returned for a share code that has been changed.
var ErrBadChecksum = errors.New("that code doesn't check out: a typo, or it was changed")

// ParseShare reads and checks a share code. It is not proof against
// cheating, but it catches typos and casual edits.
func ParseShare(code string) (Share, error) {
	var s Share
	parts := strings.Split(clean(strings.ReplaceAll(code, "—", "-")), "-")
	if len(parts) != 6 || parts[0] != "HW" {
		return s, ErrBadCode
	}
	lang, ok := words.Find(parts[1])
	if !ok {
		return s, fmt.Errorf("%w: no language %q", ErrBadCode, parts[1])
	}
	s.Lang = lang.Code
	if len(parts[2]) == seedCodeLen {
		parts[2] = fix(parts[2])
	}
	switch run := parts[2]; len(run) {
	case 4:
		m, err1 := strconv.Atoi(run[:2])
		d, err2 := strconv.Atoi(run[2:])
		if err1 != nil || err2 != nil || m < 1 || m > 12 || d < 1 || d > 31 {
			return s, ErrBadCode
		}
		s.Daily, s.Month, s.Day = true, time.Month(m), d
	case seedCodeLen:
		v, ok := decode(run)
		if !ok {
			return s, ErrBadCode
		}
		s.Seed = v
	default:
		return s, ErrBadCode
	}
	f, err := strconv.Atoi(strings.TrimPrefix(parts[3], "F"))
	if err != nil || !strings.HasPrefix(parts[3], "F") || f < 1 {
		return s, ErrBadCode
	}
	s.Floor = f
	if s.Score, err = strconv.Atoi(parts[4]); err != nil || s.Score < 0 {
		return s, ErrBadCode
	}
	if s.body() != strings.Join(parts[:5], "-") {
		return s, ErrBadCode // leading zeros and the like
	}
	if checksum(s.body()) != fix(parts[5]) {
		return s, ErrBadChecksum
	}
	return s, nil
}

// SeedFromCode reads a seed from a seed code, or from a share code of a
// run that was not a Daily Dungeon, so a friend's share code can be played
// too.
func SeedFromCode(code string) (uint64, error) {
	if seed, err := ParseSeed(code); err == nil {
		return seed, nil
	}
	s, err := ParseShare(code)
	if err != nil {
		return 0, ErrBadCode
	}
	if s.Daily {
		return 0, errors.New("that is a Daily Dungeon: play it from the Daily Dungeon")
	}
	return s.Seed, nil
}
