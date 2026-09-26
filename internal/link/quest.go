package link

import (
	"cmp"
	"fmt"
	"slices"
	"time"
)

// The goal kinds of an assignment (halpwords-server PLAN §9). A kind this
// game doesn't know is shown as just practise.
const (
	GoalMaster   = "master"   // master N words (box 3 or more)
	GoalRight    = "right"    // answer every word right N times
	GoalMinutes  = "minutes"  // practise for N minutes
	GoalFloor    = "floor"    // reach floor N in Adventure
	GoalPractise = "practise" // just practise
)

// Where a quest's answers count.
const (
	ModeAny       = "any"
	ModePractice  = "practice"
	ModeAdventure = "adventure"
)

// QuestSettings are a quest's overrides of the learner's settings for its
// list. The game doesn't apply them yet.
type QuestSettings struct {
	// Accents is a words.Strictness: 0 ignore, 1 reduced credit, 2 strict.
	Accents *int `json:"accents,omitempty"`
	// Timer is 0 normal, 1 relaxed, 2 fast.
	Timer *int `json:"timer,omitempty"`
}

// Starts is when the quest's answers start counting; zero if the server
// sent no good date.
func (q Quest) Starts() time.Time { return parseTime(q.StartsAt) }

// Due is the quest's due date, or zero when it has none.
func (q Quest) Due() time.Time { return parseTime(q.DueAt) }

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// Started reports whether the quest's answers count yet.
func (q Quest) Started(now time.Time) bool { return !now.Before(q.Starts()) }

// Complete reports whether the learner has reached the goal.
func (q Quest) Complete() bool { return q.Progress.Complete }

// kind is the goal's kind, with unknown kinds (and goals without a
// number) as just practise.
func (q Quest) kind() string {
	switch q.Goal.Kind {
	case GoalMaster, GoalRight, GoalMinutes, GoalFloor:
		if q.Goal.N > 0 {
			return q.Goal.Kind
		}
	}
	return GoalPractise
}

// HasBar reports whether the quest has a goal to fill a bar towards.
func (q Quest) HasBar() bool { return q.kind() != GoalPractise && q.Progress.Target > 0 }

// Fraction is how far the bar is filled, from 0 to 1.
func (q Quest) Fraction() float64 {
	if q.Progress.Complete {
		return 1
	}
	return float64(max(0, min(100, q.Progress.Percent))) / 100
}

// GoalText says what the quest asks for, such as "Master 20 words".
func (q Quest) GoalText() string {
	n := q.Goal.N
	switch q.kind() {
	case GoalMaster:
		return fmt.Sprintf("Master %d %s", n, plural(n, "word", "words"))
	case GoalRight:
		return fmt.Sprintf("Spell every word right %s", times(n))
	case GoalMinutes:
		return fmt.Sprintf("Practise for %d %s", n, plural(n, "minute", "minutes"))
	case GoalFloor:
		return fmt.Sprintf("Reach floor %d in an Adventure", n)
	}
	return "Practise these words"
}

// ProgressText says how far the learner has come, such as "8/20 words",
// as the server last counted it.
func (q Quest) ProgressText() string {
	p := q.Progress
	if p.Complete {
		return "Complete!"
	}
	switch q.kind() {
	case GoalMaster:
		return fmt.Sprintf("%d/%d words", p.Done, p.Target)
	case GoalRight:
		return fmt.Sprintf("%d/%d right", p.Done, p.Target)
	case GoalMinutes:
		return fmt.Sprintf("%d/%d min", p.Done, p.Target)
	case GoalFloor:
		return fmt.Sprintf("floor %d/%d", p.Done, p.Target)
	}
	if p.Answers == 0 {
		return "not started"
	}
	return fmt.Sprintf("%d %s", p.Answers, plural(p.Answers, "answer", "answers"))
}

// WhenText says when the quest is due, or when it starts if that is
// still to come, in now's time zone: "due today", "due tomorrow", "due
// Fri 2 Oct", "was due 20 Sep", "starts Mon 5 Oct", or "" when there is
// no due date.
func (q Quest) WhenText(now time.Time) string {
	if s := q.Starts(); !s.IsZero() && now.Before(s) {
		return "starts " + dayText(now, s.In(now.Location()))
	}
	d := q.Due()
	if d.IsZero() {
		return ""
	}
	d = d.In(now.Location())
	if now.After(d) {
		return "was due " + d.Format("2 Jan")
	}
	return "due " + dayText(now, d)
}

// Late reports whether the quest's due date has passed and it isn't
// complete.
func (q Quest) Late(now time.Time) bool {
	d := q.Due()
	return !d.IsZero() && now.After(d) && !q.Progress.Complete
}

// dayText names day t as seen from now: "today", "tomorrow", or a date
// such as "Fri 2 Oct".
func dayText(now, t time.Time) string {
	y1, m1, d1 := now.Date()
	y2, m2, d2 := t.Date()
	days := int(time.Date(y2, m2, d2, 12, 0, 0, 0, time.UTC).Sub(time.Date(y1, m1, d1, 12, 0, 0, 0, time.UTC)).Hours() / 24)
	switch days {
	case 0:
		return "today"
	case 1:
		return "tomorrow"
	}
	return t.Format("Mon 2 Jan")
}

// PlayModes are the ways a quest can be played, the one to offer first
// first: "practice", "adventure" or both, from where its answers count.
func (q Quest) PlayModes() []string {
	switch q.Mode {
	case ModePractice:
		return []string{ModePractice}
	case ModeAdventure:
		return []string{ModeAdventure}
	}
	if q.kind() == GoalFloor {
		return []string{ModeAdventure, ModePractice}
	}
	return []string{ModePractice, ModeAdventure}
}

// SortQuests orders quests for the game: those still to do first, the
// soonest due first and those without a due date after them, then those
// not started yet, then the complete ones. The server's order (newest
// first) breaks ties.
func SortQuests(qs []Quest, now time.Time) []Quest {
	out := slices.Clone(qs)
	group := func(q Quest) int {
		switch {
		case q.Progress.Complete:
			return 2
		case !q.Started(now):
			return 1
		}
		return 0
	}
	slices.SortStableFunc(out, func(a, b Quest) int {
		if c := cmp.Compare(group(a), group(b)); c != 0 {
			return c
		}
		da, db := a.Due(), b.Due()
		switch {
		case da.IsZero() && db.IsZero():
			return 0
		case da.IsZero():
			return 1
		case db.IsZero():
			return -1
		}
		return da.Compare(db)
	})
	return out
}

// Current is the quest to show first on the title screen: the first one
// still to do in SortQuests' order, or false when every quest is done or
// there are none.
func Current(qs []Quest, now time.Time) (Quest, bool) {
	for _, q := range SortQuests(qs, now) {
		if !q.Progress.Complete && q.Started(now) {
			return q, true
		}
	}
	return Quest{}, false
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func times(n int) string {
	switch n {
	case 1:
		return "once"
	case 2:
		return "twice"
	}
	return fmt.Sprintf("%d times", n)
}
