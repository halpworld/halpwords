package compete

import "sort"

// FameSize is how many runs each Hall of Fame table keeps.
const FameSize = 10

// Fame is one run in the Hall of Fame.
type Fame struct {
	Name  string
	Class string
	Floor int
	Score int
	Date  string // "2026-09-24"
	Code  string // the run's share code
}

// HallOfFame is the best scored runs, a table for each mode and language.
type HallOfFame struct {
	Tables map[string][]Fame
}

// TableKey names the table for runs in mode in language lang.
func TableKey(mode Mode, lang string) string {
	if mode == Daily {
		return "daily/" + lang
	}
	return "hardcore/" + lang
}

// Table returns the runs in a table, best first.
func (h *HallOfFame) Table(key string) []Fame {
	if h == nil {
		return nil
	}
	return h.Tables[key]
}

// Best returns the best score in a table, or 0.
func (h *HallOfFame) Best(key string) int {
	if t := h.Table(key); len(t) > 0 {
		return t[0].Score
	}
	return 0
}

// Rank returns the place (from 1) a score would take in a table, or 0 if
// it would not make the table.
func (h *HallOfFame) Rank(key string, score int) int {
	t := h.Table(key)
	for i, f := range t {
		if score > f.Score {
			return i + 1
		}
	}
	if len(t) < FameSize {
		return len(t) + 1
	}
	return 0
}

// Add puts a run in its table, if it is good enough, and returns its place
// (from 1), or 0 if it did not make the table. A run that ties an older one
// goes below it.
func (h *HallOfFame) Add(key string, f Fame) int {
	r := h.Rank(key, f.Score)
	if r == 0 {
		return 0
	}
	if h.Tables == nil {
		h.Tables = map[string][]Fame{}
	}
	t := append(h.Tables[key], f)
	sort.SliceStable(t, func(i, j int) bool { return t[i].Score > t[j].Score })
	if len(t) > FameSize {
		t = t[:FameSize]
	}
	h.Tables[key] = t
	return r
}
