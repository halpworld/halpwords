package settings

import "github.com/halpworld/halpwords/pkg/words"

// Source is where a setting came from. When grown-ups set the same
// setting in different places, the higher source wins: a learner's
// accommodation beats an assignment's settings, which beat a class's
// presets and the learner's settings on the website, which beat the
// player's own choice.
type Source string

// The sources, lowest first.
const (
	// Own is the player's own choice in the game.
	Own Source = "own"
	// Learner is the learner's settings on the website, set by a parent
	// or teacher.
	Learner Source = "learner"
	// Class is a class's presets, set by its teacher.
	Class Source = "class"
	// Assignment is an assignment's settings, for its list only.
	Assignment Source = "assignment"
	// Accommodation is an accommodation a grown-up turned on for the
	// learner (relaxed timers, accents ignored).
	Accommodation Source = "accommodation"
)

// rank orders the sources. The learner's settings and a class's presets
// rank the same, so the stricter of them wins.
func (s Source) rank() int {
	switch s {
	case Learner, Class:
		return 1
	case Assignment:
		return 2
	case Accommodation:
		return 3
	}
	return 0
}

// Adult reports whether a grown-up set it (anything but Own).
func (s Source) Adult() bool { return s.rank() > 0 }

// Decider is who decided a setting: the source and the role of the
// grown-up ("teacher", "guardian"), empty for Own.
type Decider struct {
	Source Source `json:"source"`
	Role   string `json:"role,omitempty"`
}

// Decided says who decided the settings a grown-up can set.
type Decided struct {
	Accents Decider `json:"accents"`
	Timer   Decider `json:"timer"`
}

// Roles are the roles of the grown-ups who decided, each once, accents
// first.
func (d Decided) Roles() []string {
	var out []string
	for _, x := range []Decider{d.Accents, d.Timer} {
		if x.Source.Adult() && x.Role != "" && (len(out) == 0 || out[0] != x.Role) {
			out = append(out, x.Role)
		}
	}
	return out
}

// Adult reports whether a grown-up decided any of them.
func (d Decided) Adult() bool { return d.Accents.Source.Adult() || d.Timer.Source.Adult() }

// Layer is one place a grown-up set accents or the timer. Nil leaves the
// setting as it is.
type Layer struct {
	Source Source
	Role   string
	// Accents is how strictly accents are marked; in Ancient Greek it
	// sets breathings too.
	Accents *words.Strictness
	Timer   *Timer
}

// timerRank orders timers from the most lenient: relaxed, normal, fast.
func timerRank(t Timer) int {
	switch t {
	case Relaxed:
		return 0
	case Fast:
		return 2
	}
	return 1
}

// StricterTimer reports whether a gives less time than b.
func StricterTimer(a, b Timer) bool { return timerRank(a) > timerRank(b) }

// wins reports whether a layer's value replaces the current one: it
// comes from a higher source, or from the same source and is at least
// as strict (so the strictest grown-up's setting applies, and says who
// set it).
func wins(l Source, cur Source, atLeastAsStrict bool) bool {
	if l.rank() != cur.rank() {
		return l.rank() > cur.rank()
	}
	return l.Adult() && atLeastAsStrict
}

// Resolve applies the layers to base, whose settings were decided by
// by, and returns the settings to play with and who decided them. For
// each setting the highest source wins, and among layers of the same
// source the strictest. The layers' order doesn't matter.
func Resolve(lang *words.Language, base Lang, by Decided, layers ...Layer) (Lang, Decided) {
	if by.Accents.Source == "" {
		by.Accents.Source = Own
	}
	if by.Timer.Source == "" {
		by.Timer.Source = Own
	}
	for _, l := range layers {
		if a := l.Accents; a != nil && wins(l.Source, by.Accents.Source, *a >= base.Rules.Accents) {
			base.Rules.Accents = *a
			if lang != nil && lang.Script == words.ScriptGreek {
				base.Rules.Breathings = *a
			}
			by.Accents = Decider{Source: l.Source, Role: l.Role}
		}
		if t := l.Timer; t != nil && t.Valid() && wins(l.Source, by.Timer.Source, !StricterTimer(base.Timer, *t)) {
			base.Timer = *t
			by.Timer = Decider{Source: l.Source, Role: l.Role}
		}
	}
	return base, by
}
