package move

import (
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/halpworld/halpwords/internal/save"
)

// Store is where the game keeps its files: the save package, or a map in
// tests.
type Store interface {
	Read(name string) ([]byte, error)
	Write(name string, data []byte) error
	Remove(name string) error
	All() ([]string, error) // every file name, in every folder
}

// Saves is the game's own store.
var Saves Store = saveStore{}

type saveStore struct{}

func (saveStore) Read(name string) ([]byte, error)     { return save.Read(name) }
func (saveStore) Write(name string, data []byte) error { return save.Write(name, data) }
func (saveStore) Remove(name string) error             { return save.Remove(name) }
func (saveStore) All() ([]string, error)               { return save.All() }

// The package keeps parts that have arrived, and the IDs of payloads
// already dealt with, in the stash folder. Nothing in it moves.
const (
	stashDir = "move/"
	doneFile = stashDir + "done"
)

func partFile(p Part) string {
	return fmt.Sprintf("%spart-%s-%d-%d", stashDir, p.ID, p.K, p.N)
}

// Incoming is progress that has arrived from the old address, waiting for
// the player to say whether it may replace what is here.
type Incoming struct {
	ID    string
	Files map[string][]byte
}

// Receive handles the page's URL fragment when the game starts: it keeps
// the part the fragment carries, if any. Then:
//   - if a part of the payload is still missing, next is its number, and
//     the game should fetch it from OldURL#send=next;
//   - if every part is here, in is the progress, ready to Apply or Discard;
//   - otherwise both are zero, and the game just starts.
//
// A payload that was already applied or discarded is ignored, so an old
// link followed again doesn't ask a second time.
func Receive(st Store, frag string) (in *Incoming, next int, err error) {
	part, ok, err := ParseFragment(frag)
	if err != nil {
		return nil, 0, err
	}
	if ok {
		if done(st, part.ID) {
			return nil, 0, nil
		}
		if err := st.Write(partFile(part), []byte(part.Data)); err != nil {
			return nil, 0, err
		}
	}
	parts, err := stashed(st)
	if err != nil || len(parts) == 0 {
		return nil, 0, err
	}
	// Keep only the payload that arrived last (or the one in the stash).
	id := parts[0].ID
	if ok {
		id = part.ID
	}
	var mine []Part
	for _, p := range parts {
		if p.ID == id {
			mine = append(mine, p)
		} else {
			st.Remove(partFile(p)) // an older, unfinished move
		}
	}
	for k := 1; k <= mine[0].N; k++ {
		if !slices.ContainsFunc(mine, func(p Part) bool { return p.K == k }) {
			if !ok {
				// A move left unfinished: fetch parts only while one is
				// arriving, so the game never keeps bouncing to the old page.
				return nil, 0, nil
			}
			return nil, k, nil
		}
	}
	payload, err := Join(mine)
	if err == nil {
		var files map[string][]byte
		if files, err = Decode(payload); err == nil {
			return &Incoming{ID: id, Files: files}, 0, nil
		}
	}
	(&Incoming{ID: id}).Discard(st)
	return nil, 0, err
}

// stashed returns the parts kept so far.
func stashed(st Store) ([]Part, error) {
	names, err := st.All()
	if err != nil {
		return nil, err
	}
	var parts []Part
	for _, name := range names {
		rest, ok := strings.CutPrefix(name, stashDir+"part-")
		if !ok {
			continue
		}
		var p Part
		if _, err := fmt.Sscanf(strings.ReplaceAll(rest, "-", " "), "%s %d %d", &p.ID, &p.K, &p.N); err != nil {
			st.Remove(name)
			continue
		}
		data, err := st.Read(name)
		if err != nil {
			return nil, err
		}
		p.Data = string(data)
		parts = append(parts, p)
	}
	return parts, nil
}

func done(st Store, id string) bool {
	data, err := st.Read(doneFile)
	return err == nil && slices.Contains(strings.Fields(string(data)), id)
}

// Conflicts returns the files already here that moving would replace or
// remove: every file that moves. With none, the game can Apply without
// asking.
func Conflicts(st Store) ([]string, error) {
	names, err := st.All()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, n := range names {
		if Exportable(n) {
			out = append(out, n)
		}
	}
	return out, nil
}

// Apply replaces the progress here with the incoming progress: files here
// that move and are not in the incoming progress are removed, so the hero,
// word memory and lists all come from one place.
func (in *Incoming) Apply(st Store) error {
	here, err := Conflicts(st)
	if err != nil {
		return err
	}
	for _, name := range here {
		if _, ok := in.Files[name]; !ok {
			if err := st.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
	}
	names := make([]string, 0, len(in.Files))
	for name := range in.Files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if err := st.Write(name, in.Files[name]); err != nil {
			return err
		}
	}
	return in.Discard(st)
}

// Discard forgets the incoming progress and remembers that it was dealt
// with.
func (in *Incoming) Discard(st Store) error {
	parts, _ := stashed(st)
	for _, p := range parts {
		if p.ID == in.ID {
			st.Remove(partFile(p))
		}
	}
	data, _ := st.Read(doneFile)
	ids := strings.Fields(string(data))
	if slices.Contains(ids, in.ID) {
		return nil
	}
	ids = append(ids, in.ID)
	if len(ids) > 20 {
		ids = ids[len(ids)-20:]
	}
	return st.Write(doneFile, []byte(strings.Join(ids, "\n")+"\n"))
}

// SendURL is the old page's address that sends part k.
func SendURL(k int) string { return fmt.Sprintf("%s#%s%d", OldURL, SendKey, k) }
