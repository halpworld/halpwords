// Package race holds the rules of a Race (halpwords-server's play rooms,
// its docs/api/play.md): two to four racers play their own copy of the
// same dungeon, from the same seed and word list, and the first to reach
// floor Goal wins. Each game reports its floor, the monsters it has
// beaten and where the hero stands; the server checks every report with a
// Judge against what the game can physically do, and leaves racers whose
// reports are impossible out of the results.
//
// The numbers here are the game's own: a step takes StepTime, a monster
// takes MonsterTime to fall, and floors are the size MapSize says. The
// dungeon's tests check them against thousands of generated floors, so a
// change to the dungeon that breaks them fails there first.
//
// Like the other pkg/ packages it has no Ebitengine dependency.
package race

import (
	"fmt"
	"time"
)

// Goal is the floor a race is run to: the first racer to reach it wins.
const Goal = 3

// TimeLimit is how long a race lasts at most. When it is up, racers are
// ranked by how far they got.
const TimeLimit = 10 * time.Minute

// StepTime is the shortest time the hero takes to walk one cell: the
// crawl's step animation, 9 ticks at 60 a second.
const StepTime = 150 * time.Millisecond

// MinStairsSteps is fewer than the fewest cells between a floor's start
// and its stairs (the dungeon's tests find at least 9 on floor 1 over
// 20,000 floors of each depth; this leaves room for rarer floors).
const MinStairsSteps = 6

// MonsterTime is the shortest time a monster takes to beat: it must be
// hit by at least one typed answer, and its fall takes half a second.
const MonsterTime = 500 * time.Millisecond

// MapSize is the width and height of floor, in cells. It is the
// dungeon's size for the floor (checked by the dungeon's tests).
func MapSize(floor int) int { return min(24+floor*2, 40) }

// MaxMonsters is more than the most monsters one floor can have: those
// placed on it, a boss, and a mimic in every chest.
func MaxMonsters(floor int) int { return 3 + floor + 1 + 3 }

// Report is what a racing game tells the server: the floor the hero is
// on (from 1), the monsters beaten in the race so far, the hero's cell,
// and whether the hero has fallen (which ends their race).
type Report struct {
	Floor    int  `json:"floor"`
	Monsters int  `json:"monsters"`
	X        int  `json:"x"`
	Y        int  `json:"y"`
	Fell     bool `json:"fell,omitempty"`
}

// Finished reports whether the racer has reached the goal.
func (r Report) Finished() bool { return r.Floor >= Goal }

// Over reports whether the racer's race is over: they reached the goal
// or fell.
func (r Report) Over() bool { return r.Finished() || r.Fell }

// Why a report is implausible (Implausible.Reason).
const (
	// ReasonFloor: a floor out of order (back up, or two down at once)
	// or out of range.
	ReasonFloor = "floor"
	// ReasonTooSoon: a floor reached sooner than walking there takes.
	ReasonTooSoon = "too-soon"
	// ReasonMonsters: fewer monsters than before, more than the floors
	// hold, or more than there was time to beat.
	ReasonMonsters = "monsters"
	// ReasonPosition: a cell outside the floor.
	ReasonPosition = "position"
	// ReasonSpeed: the hero moved further than there was time to walk,
	// too often.
	ReasonSpeed = "speed"
	// ReasonOver: a report that changes something after the racer's
	// race was over.
	ReasonOver = "over"
)

// Implausible is a report the game can't have made. The server leaves
// the racer out of the results.
type Implausible struct {
	Reason string
}

func (e *Implausible) Error() string { return "race: implausible report (" + e.Reason + ")" }

// Movement is checked with a budget of steps that fills at one step per
// StepTime, up to MoveSlack's worth, so reports that arrive late and
// bunched together (a slow network) are not mistaken for a fast hero.
// A report over the budget is a strike; MaxStrikes of them are
// implausible. Monsters are checked the same way.
const (
	// MoveSlack is how late reports may arrive, in the budgets.
	MoveSlack = 6 * time.Second
	// MaxStrikes is how many reports over a budget a racer may send.
	MaxStrikes = 3
)

// Judge follows one racer's reports and says when one is implausible.
// The zero Judge is ready for the first report. It is not safe for
// concurrent use.
type Judge struct {
	last    Report
	lastAt  time.Duration
	started bool
	// floorMonsters is the monster count when the racer reached their
	// floor, and floorAt when that was.
	floorMonsters int
	floorAt       time.Duration
	steps, kills  float64 // the budgets
	strikes       int
}

// Last returns the last plausible report (the zero Report before any).
func (j *Judge) Last() Report { return j.last }

// Check judges report r, which arrived at time at since the race
// started (by the server's clock, so a network delay only makes at
// later). It returns an *Implausible for a report the game can't have
// made; a plausible report becomes Last. Time-from-start checks need no
// slack: a game starts no sooner than the server says, so it can only
// be behind.
func (j *Judge) Check(r Report, at time.Duration) error {
	if !j.started {
		j.last = Report{Floor: 1}
		j.steps, j.kills = budget(MoveSlack, StepTime), budget(MoveSlack, MonsterTime)
	}
	prev := j.last
	switch {
	case prev.Over():
		if r != prev {
			return &Implausible{ReasonOver}
		}
		return nil
	case r.Floor < prev.Floor || r.Floor > prev.Floor+1 || r.Floor < 1 || r.Floor > Goal:
		return &Implausible{ReasonFloor}
	case at < time.Duration(r.Floor-1)*MinStairsSteps*StepTime:
		return &Implausible{ReasonTooSoon}
	case r.Monsters < prev.Monsters || at < time.Duration(r.Monsters)*MonsterTime:
		return &Implausible{ReasonMonsters}
	case r.X < 0 || r.Y < 0 || r.X >= MapSize(r.Floor) || r.Y >= MapSize(r.Floor):
		return &Implausible{ReasonPosition}
	}
	newFloor := r.Floor > prev.Floor
	if newFloor {
		// Monsters beaten on the floor just left and the new one.
		if r.Monsters-j.floorMonsters > MaxMonsters(prev.Floor)+MaxMonsters(r.Floor) {
			return &Implausible{ReasonMonsters}
		}
	} else if r.Monsters-j.floorMonsters > MaxMonsters(r.Floor) {
		return &Implausible{ReasonMonsters}
	}
	dt := at - j.lastAt
	steps := min(j.steps+float64(dt)/float64(StepTime), budget(MoveSlack, StepTime))
	kills := min(j.kills+float64(dt)/float64(MonsterTime), budget(MoveSlack, MonsterTime))
	moved := 0
	if !newFloor && j.started {
		// A new floor starts the hero somewhere new: only moves on one
		// floor are walked.
		moved = abs(r.X-prev.X) + abs(r.Y-prev.Y)
	}
	strike := false
	if float64(moved) > steps+1 {
		strike, steps = true, 0
	} else {
		steps = max(0, steps-float64(moved))
	}
	if beaten := float64(r.Monsters - prev.Monsters); beaten > kills+1 {
		strike, kills = true, 0
	} else {
		kills = max(0, kills-beaten)
	}
	// Two floors in less than half the shortest walk between them. (The
	// time from the start, above, needs no slack; this does.)
	if newFloor && at-j.floorAt < MinStairsSteps*StepTime/2 {
		strike = true
	}
	if strike {
		j.strikes++
		if j.strikes >= MaxStrikes {
			return &Implausible{ReasonSpeed}
		}
	}
	if newFloor {
		j.floorMonsters, j.floorAt = prev.Monsters, at
	}
	j.last, j.lastAt, j.started = r, at, true
	j.steps, j.kills = steps, kills
	return nil
}

// budget is how many things taking each fit in d.
func budget(d, each time.Duration) float64 { return float64(d) / float64(each) }

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// ReportEvery is the least time between two reports that only move the
// hero or count a monster; a new floor or a fall is reported at once.
const ReportEvery = 500 * time.Millisecond

// Reporter decides when a racing game sends a report: when something
// changed, at most every ReportEvery, and at once for a new floor or a
// fall. Ask it every frame, so the last change always goes out. The zero
// Reporter is ready.
type Reporter struct {
	last Report
	sent time.Time
	any  bool
}

// Due reports whether r should be sent now, at now. When it says yes it
// counts r as sent.
func (p *Reporter) Due(r Report, now time.Time) bool {
	switch {
	case p.any && r == p.last:
		return false
	case p.any && r.Floor == p.last.Floor && r.Fell == p.last.Fell && now.Sub(p.sent) < ReportEvery:
		return false
	}
	p.last, p.sent, p.any = r, now, true
	return true
}

// String is a report for logs and tests.
func (r Report) String() string {
	s := fmt.Sprintf("floor %d, %d monsters, at %d,%d", r.Floor, r.Monsters, r.X, r.Y)
	if r.Fell {
		s += ", fell"
	}
	return s
}
