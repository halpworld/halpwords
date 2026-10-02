package scene

import (
	"cmp"
	"slices"
	"sync"
	"time"

	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/link"
	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/pkg/words"
)

// listPool is which of a language's word lists a run or a practice deals
// from when neither an assignment nor a quest names its lists (#89).
type listPool struct {
	// starters is the built-in lists only, never a copy the player
	// edited: Daily and Hardcore runs, so scores stay comparable.
	starters bool
	// keys are the lists ticked on the checklist (listKey); nil is every
	// list.
	keys []string
}

// lists returns the pool's lists in lang, in the order the game keeps
// them (not the order they were ticked), so the same lists always deal
// the same words from the same seed. Lists that are gone are dropped;
// when none is left, it is every list.
func (p listPool) lists(ctx *game.Context, lang *words.Language) []*words.List {
	if p.starters {
		if s := starterLists(lang); len(s) > 0 {
			return s
		}
	}
	all := ctx.ListsFor(lang.Code)
	if p.keys == nil {
		return all
	}
	var out []*words.List
	for _, l := range all {
		if slices.Contains(p.keys, listKey(l)) {
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		return all
	}
	return out
}

// listKey names a word list on the checklist and in the learner's
// settings: a list from the server by its id, any other by its file.
func listKey(l *words.List) string {
	if link.IsAssigned(l) && l.ID != "" {
		return l.ID
	}
	return "file:" + l.File
}

// keysOf returns the keys of lists, in order.
func keysOf(lists []*words.List) []string {
	keys := make([]string, 0, len(lists))
	for _, l := range lists {
		keys = append(keys, listKey(l))
	}
	return keys
}

// starters are the built-in word lists, read once.
var starters struct {
	once  sync.Once
	lists []*words.List
}

// starterLists returns the built-in word lists for lang, as the game
// ships them: a list in the words folder with a starter's file name is
// not one of them. Daily and Hardcore runs play these.
func starterLists(lang *words.Language) []*words.List {
	starters.once.Do(func() {
		starters.lists, _ = game.StarterLists() // embedded; checked by tests
	})
	var out []*words.List
	for _, l := range starters.lists {
		if l.Language == lang.Code {
			out = append(out, l)
		}
	}
	return out
}

// lockedLists returns the lists in lang that an open assignment locks
// the learner to ("Lock to this list"), or nil.
func lockedLists(ctx *game.Context, lang *words.Language) []*words.List {
	var out []*words.List
	for _, l := range ctx.ListsFor(lang.Code) {
		if li, ok := ctx.Link.Info(l); ok && li.Locked {
			out = append(out, l)
		}
	}
	return out
}

// pickedPool is the pool Adventure and Practice play in lang without
// the checklist being shown again: the locked lists, if an assignment
// locks some, or else the lists the learner ticked last (every list when
// they never ticked any).
func pickedPool(ctx *game.Context, lang *words.Language) listPool {
	if locked := lockedLists(ctx, lang); len(locked) > 0 {
		return listPool{keys: keysOf(locked)}
	}
	if ctx.Profile == nil {
		return listPool{}
	}
	keys, ok := ctx.Profile.Settings.Lists.Picked(lang.Code)
	if !ok {
		return listPool{}
	}
	return listPool{keys: keys}
}

// listRow is a word list as the checklist shows it.
type listRow struct {
	list   *words.List
	key    string
	ticked bool
	locked bool
	sent   bool // sent to the game by a grown-up
	assign bool // an assignment's list
	isNew  bool // a sent list shown for the first time
	when   time.Time
}

// listRows lays out the word lists in lang for the checklist, newest
// first, ticked as pickedPool would play them. A sent list the learner
// has not been told about is ticked once and marked new; it is then
// remembered as seen, in the learner's settings.
func listRows(ctx *game.Context, lang *words.Language) []*listRow {
	all := ctx.ListsFor(lang.Code)
	locked := lockedLists(ctx, lang)
	pool := pickedPool(ctx, lang)
	starts := questStarts(ctx)
	rows := make([]*listRow, 0, len(all))
	changed, keys := false, slices.Clone(pool.keys)
	for _, l := range all {
		r := &listRow{list: l, key: listKey(l)}
		r.ticked = pool.keys == nil || slices.Contains(pool.keys, r.key)
		if li, ok := ctx.Link.Info(l); ok {
			r.locked = li.Locked
			r.sent = li.Source == link.SourceSent
			r.assign = !r.sent
			r.when = li.SentAt
			if !r.sent {
				r.when = starts[li.ID]
			}
			if r.sent && ctx.Profile != nil && !ctx.Profile.Settings.Lists.WasSeen(li.ID) {
				r.isNew = true
				ctx.Profile.Settings.Lists.See(li.ID)
				changed = true
				if len(locked) == 0 && !r.ticked {
					r.ticked = true
					keys = append(keys, r.key)
				}
			}
		} else if !save.InMemory() {
			r.when = save.ModTime(game.WordsDir + "/" + l.File) // zero for a starter
		}
		if len(locked) > 0 {
			r.ticked = r.locked
		}
		rows = append(rows, r)
	}
	if changed {
		if pool.keys != nil && len(locked) == 0 {
			ctx.Profile.Settings.Lists.Pick(lang.Code, keys)
		}
		ctx.Profile.SaveSettings()
	}
	// Newest first. Lists with no time keep the game's order backwards:
	// lists from the server, then the player's own, then the starters.
	order := map[*listRow]int{}
	for i, r := range rows {
		order[r] = i
	}
	slices.SortStableFunc(rows, func(a, b *listRow) int {
		if c := b.when.Compare(a.when); c != 0 {
			return c
		}
		return cmp.Compare(order[b], order[a])
	})
	return rows
}

// questStarts returns when each assigned list's newest assignment
// started, by list id.
func questStarts(ctx *game.Context) map[string]time.Time {
	out := map[string]time.Time{}
	for _, q := range ctx.Link.Quests() {
		if s := q.Starts(); s.After(out[q.List.ID]) {
			out[q.List.ID] = s
		}
	}
	return out
}
