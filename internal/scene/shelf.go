package scene

import (
	"slices"
	"strconv"
	"strings"

	"github.com/halpworld/halpwords/internal/game"
	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/pkg/words"
)

// shelfRow is one word list on the Word Lists screen.
type shelfRow struct {
	file    string      // file name, such as "french.txt"
	list    *words.List // the words as they are now, saved or not
	starter *words.List // the built-in list with this file name, if any
	own     bool        // the user has their own copy (saved or not)
	onDisk  bool        // the user's copy is in their words folder
	dirty   bool        // changed since the last save
	marked  bool        // picked for deleting
}

// deletable reports whether the row can be deleted. A starter list can't,
// but the user's edits to one can be undone.
func (r *shelfRow) deletable() bool { return r.own }

// shelf holds every word list while the user sorts them out. Changes stay
// in memory until save.
type shelf struct {
	rows   []*shelfRow
	remove map[string]bool // files to delete from the words folder on save
}

// newShelf lays out the starter lists and the user's own lists. A user list
// with a starter's file name is the user's edited copy of it.
func newShelf(starters, user []*words.List) *shelf {
	s := &shelf{remove: map[string]bool{}}
	for _, l := range starters {
		s.rows = append(s.rows, &shelfRow{file: l.Source, list: l, starter: l})
	}
	for _, l := range user {
		if r := s.find(l.Source); r != nil {
			r.list, r.own, r.onDisk = l, true, true
			continue
		}
		s.rows = append(s.rows, &shelfRow{file: l.Source, list: l, own: true, onDisk: true})
	}
	return s
}

func (s *shelf) find(file string) *shelfRow {
	for _, r := range s.rows {
		if strings.EqualFold(r.file, file) {
			return r
		}
	}
	return nil
}

// unsaved reports whether anything has changed since the last save.
func (s *shelf) unsaved() bool {
	return len(s.remove) > 0 || slices.ContainsFunc(s.rows, func(r *shelfRow) bool { return r.dirty })
}

// targets returns the rows an import in language lang can be added to.
func (s *shelf) targets(lang string) []int {
	var out []int
	for i, r := range s.rows {
		if r.list.Language == lang {
			out = append(out, i)
		}
	}
	return out
}

// create adds l as a new list and returns its row number.
func (s *shelf) create(l *words.List) int {
	base := strings.TrimSuffix(words.FileName(l.Title), ".txt")
	file := base + ".txt"
	for n := 2; s.find(file) != nil; n++ {
		file = base + "-" + strconv.Itoa(n) + ".txt"
	}
	nl := cloneList(l)
	nl.Source = file
	s.rows = append(s.rows, &shelfRow{file: file, list: nl, own: true, dirty: true})
	return len(s.rows) - 1
}

// add adds the words in l to row i and returns how many were new.
func (s *shelf) add(i int, l *words.List) int {
	r := s.rows[i]
	if !r.own {
		r.list, r.own = cloneList(r.list), true // never change the starter
	}
	added := r.list.Merge(l)
	r.dirty = true
	return added
}

// deleteRows deletes the lists in rows idx, or turns starter lists back into
// the built-in version. It returns how many were deleted and reset.
func (s *shelf) deleteRows(idx []int) (deleted, reset int) {
	gone := map[*shelfRow]bool{}
	for _, i := range idx {
		r := s.rows[i]
		if !r.deletable() {
			continue
		}
		if r.onDisk {
			s.remove[r.file] = true
		}
		if r.starter != nil {
			r.list, r.own, r.onDisk, r.dirty = r.starter, false, false, false
			reset++
		} else {
			gone[r] = true
			deleted++
		}
		r.marked = false
	}
	s.rows = slices.DeleteFunc(s.rows, func(r *shelfRow) bool { return gone[r] })
	return deleted, reset
}

// save writes every changed list to the words folder and deletes the ones
// that were deleted. It stops at the first error.
func (s *shelf) save() error {
	for _, r := range s.rows {
		if !r.own || !r.dirty {
			continue
		}
		if err := save.Write(game.WordsDir+"/"+r.file, r.list.Format()); err != nil {
			return err
		}
		r.dirty, r.onDisk = false, true
		delete(s.remove, r.file)
	}
	for f := range s.remove {
		if err := save.Remove(game.WordsDir + "/" + f); err != nil {
			return err
		}
		delete(s.remove, f)
	}
	return nil
}

func cloneList(l *words.List) *words.List {
	c := *l
	c.Entries = make([]words.Entry, len(l.Entries))
	for i, e := range l.Entries {
		e.Answers = slices.Clone(e.Answers)
		c.Entries[i] = e
	}
	return &c
}
