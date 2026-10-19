// Package save keeps the game's files, such as the saved adventure. On the
// desktop they are files in the user's config folder; in a web browser they
// go in the page's local storage; while a quest is play-tested they are
// only kept in memory (UseMemory). Files are small, so they are always read
// and written whole, and Read fails with an error matching fs.ErrNotExist
// when there is no such file. Names use forward slashes, such as
// "words/animals.txt", on every system.
package save

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// memory holds every file while the game keeps its files in memory (see
// UseMemory); it is nil otherwise.
var (
	memMu  sync.Mutex
	memory map[string][]byte
)

// UseMemory keeps every file in memory from now on, starting with none:
// nothing is read from or written to the user's folder or the browser's
// local storage again until the game closes. The web game's play-test
// mode uses it, so trying out a quest never touches a player's saves.
func UseMemory() {
	memMu.Lock()
	defer memMu.Unlock()
	if memory == nil {
		memory = map[string][]byte{}
	}
}

// InMemory reports whether files are kept in memory (UseMemory).
func InMemory() bool {
	memMu.Lock()
	defer memMu.Unlock()
	return memory != nil
}

// errNoDir is Dir's error while files are kept in memory.
var errNoDir = errors.New("no user folder: files are kept in memory")

// Folder is a folder of files inside the user's folder, such as
// "profiles/k3v9q2" for one learner's files (W2.5). Its methods take names
// inside it. Root is the user's folder itself.
type Folder string

// Root is the user's folder, for the files every learner on the computer
// shares: the learners' list, the AI helper's settings, crash reports.
const Root Folder = ""

// current is the folder the package's functions use: the learner playing
// now. It starts as Root.
var (
	curMu   sync.Mutex
	current Folder
)

// Use makes f the folder the package's functions (Read, Write and so on)
// use from now on. Code that keeps writing after the learner changes,
// such as the link's background sync, holds a Folder instead.
func Use(f Folder) {
	curMu.Lock()
	defer curMu.Unlock()
	current = f
}

// Current is the folder the package's functions use.
func Current() Folder {
	curMu.Lock()
	defer curMu.Unlock()
	return current
}

// name is the file's name in the user's folder.
func (f Folder) name(n string) string {
	if f == "" {
		return n
	}
	return string(f) + "/" + n
}

// Dir is where saves, settings and the user's own word lists live, e.g.
// ~/Library/Application Support/halpwords on macOS, or the current
// folder inside it. It fails in a web browser, which has only local
// storage, and while files are kept in memory.
func Dir() (string, error) { return Current().Dir() }

// Dir is where the folder's files live on disk. It fails in a web
// browser and while files are kept in memory.
func (f Folder) Dir() (string, error) {
	if InMemory() {
		return "", errNoDir
	}
	d, err := diskDir()
	if err != nil || f == "" {
		return d, err
	}
	return filepath.Join(d, filepath.FromSlash(string(f))), nil
}

// Read returns the contents of the named file in the current folder.
func Read(name string) ([]byte, error) { return Current().Read(name) }

// Write replaces the named file in the current folder.
func Write(name string, data []byte) error { return Current().Write(name, data) }

// WritePrivate replaces the named file in the current folder with one
// only the user can read, for secrets such as API keys.
func WritePrivate(name string, data []byte) error { return Current().WritePrivate(name, data) }

// Remove deletes the named file in the current folder.
func Remove(name string) error { return Current().Remove(name) }

// ModTime returns when the named file in the current folder was last
// written, or the zero time where that is not known (see Folder.ModTime).
func ModTime(name string) time.Time { return Current().ModTime(name) }

// List returns the names of the files in the folder dir of the current
// folder.
func List(dir string) ([]string, error) { return Current().List(dir) }

// All returns the names of every file in the current folder, in every
// folder inside it, sorted, with forward slashes.
func All() ([]string, error) { return Current().All() }

// Read returns the contents of the named file.
func (f Folder) Read(name string) ([]byte, error) {
	name = f.name(name)
	memMu.Lock()
	defer memMu.Unlock()
	if memory == nil {
		return diskRead(name)
	}
	data, ok := memory[name]
	if !ok {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrNotExist}
	}
	return append([]byte(nil), data...), nil
}

// Write replaces the named file. On the desktop it writes a temporary
// file first, so a crash part way through never leaves half a save.
//
// The write is durable: the file is flushed to disk before the rename, so a
// power cut leaves the old file or the new one. That costs about 10 ms on
// an SSD, more on a hard disk; use WriteFast for what is written often.
func (f Folder) Write(name string, data []byte) error { return f.write(name, data, true) }

// WriteFast is Write without the flush to disk: still a temporary file and
// a rename, so a crash never leaves half a file, but a power cut can lose
// the newest write or leave an empty file. It is for what is written on
// every answer (word memory) until the writes are debounced (#53 item 5).
// Settings, saves, learners.json and the link's files use Write.
func (f Folder) WriteFast(name string, data []byte) error { return f.write(name, data, false) }

func (f Folder) write(name string, data []byte, durable bool) error {
	name = f.name(name)
	memMu.Lock()
	defer memMu.Unlock()
	if memory == nil {
		return diskWrite(name, data, durable)
	}
	memory[name] = append([]byte(nil), data...)
	return nil
}

// WritePrivate replaces the named file with one only the user can read, for
// secrets such as API keys.
func (f Folder) WritePrivate(name string, data []byte) error {
	if InMemory() {
		return f.Write(name, data)
	}
	return diskWritePrivate(f.name(name), data)
}

// Remove deletes the named file. Removing a file that is not there is not
// an error.
func (f Folder) Remove(name string) error {
	name = f.name(name)
	memMu.Lock()
	defer memMu.Unlock()
	if memory == nil {
		return diskRemove(name)
	}
	delete(memory, name)
	return nil
}

// KeepBad is how many damaged copies of one file Quarantine keeps.
const KeepBad = 3

// Quarantine keeps a damaged file's contents to one side, as
// "<name>.<UTC time>.bad", so a second damage never overwrites the first
// copy. It keeps the newest KeepBad copies of that file (the older
// "<name>.bad" of earlier versions counts as the oldest) and removes the
// rest. It returns the name it wrote.
func (f Folder) Quarantine(name string, data []byte) (string, error) {
	return f.quarantine(name, data, time.Now().UTC())
}

func (f Folder) quarantine(name string, data []byte, at time.Time) (string, error) {
	dir, base := path.Split(name)
	stamp := at.Format("20060102T150405Z")
	existing, err := f.List(dir)
	if err != nil {
		return "", err
	}
	have := map[string]bool{}
	for _, n := range existing {
		have[n] = true
	}
	bad := base + "." + stamp + ".bad"
	for i := 2; have[bad]; i++ {
		bad = fmt.Sprintf("%s.%s-%d.bad", base, stamp, i)
	}
	if err := f.Write(dir+bad, data); err != nil {
		return "", err
	}
	have[bad] = true
	var copies []string // oldest first: the old fixed name, then by time
	if have[base+".bad"] {
		copies = append(copies, base+".bad")
	}
	type stamped struct{ name, key string }
	var ours []stamped
	for n := range have {
		if k, ok := badKey(base, n); ok {
			ours = append(ours, stamped{n, k})
		}
	}
	sort.Slice(ours, func(i, j int) bool { return ours[i].key < ours[j].key })
	for _, o := range ours {
		copies = append(copies, o.name)
	}
	// Trim the oldest, but never the copy just written, even when the
	// clock is behind and it sorts first.
	for i := 0; len(copies) > KeepBad && i < len(copies); {
		if copies[i] == bad {
			i++
			continue
		}
		if err := f.Remove(dir + copies[i]); err != nil {
			return dir + bad, err
		}
		copies = append(copies[:i], copies[i+1:]...)
	}
	return dir + bad, nil
}

// badKey reports whether n is a copy Quarantine made of the file base,
// "<base>.<stamp>[-k].bad", and a key that sorts copies oldest first.
func badKey(base, n string) (string, bool) {
	mid, ok := strings.CutPrefix(n, base+".")
	if !ok {
		return "", false
	}
	mid, ok = strings.CutSuffix(mid, ".bad")
	if !ok || len(mid) < 16 || mid[8] != 'T' || mid[15] != 'Z' {
		return "", false
	}
	k := 1
	if rest := mid[16:]; rest != "" {
		if _, err := fmt.Sscanf(rest, "-%d", &k); err != nil {
			return "", false
		}
	}
	return fmt.Sprintf("%s%09d", mid[:16], k), true
}

// ModTime returns when the named file was last written. It is the zero
// time for a file that is not there, in a web browser (local storage
// keeps no times) and while files are kept in memory.
func (f Folder) ModTime(name string) time.Time {
	if InMemory() {
		return time.Time{}
	}
	return diskModTime(f.name(name))
}

// List returns the names of the files in the folder dir, sorted. A folder
// that is not there is empty.
func (f Folder) List(dir string) ([]string, error) {
	dir = strings.TrimSuffix(f.name(strings.TrimSuffix(dir, "/")), "/")
	memMu.Lock()
	defer memMu.Unlock()
	if memory == nil {
		return diskList(dir)
	}
	var names []string
	for name := range memory {
		if d := path.Dir(name); d == dir || (d == "." && dir == "") {
			names = append(names, path.Base(name))
		}
	}
	sort.Strings(names)
	return names, nil
}

// All returns the names of every file in the folder, in every folder
// inside it, sorted, with forward slashes. A missing folder is empty.
func (f Folder) All() ([]string, error) {
	var all []string
	memMu.Lock()
	if memory == nil {
		memMu.Unlock()
		var err error
		if all, err = diskAll(); err != nil {
			return nil, err
		}
	} else {
		for name := range memory {
			all = append(all, name)
		}
		memMu.Unlock()
		sort.Strings(all)
	}
	if f == "" {
		return all, nil
	}
	var names []string
	for _, n := range all {
		if rest, ok := strings.CutPrefix(n, string(f)+"/"); ok {
			names = append(names, rest)
		}
	}
	return names, nil
}

// RemoveAll deletes every file in the folder, and the folder itself on
// disk. It stays inside the user's folder: a folder that is a symbolic
// link is unlinked, never followed, and one that resolves to somewhere
// outside the user's folder is refused.
func (f Folder) RemoveAll() error {
	if f != Root && !InMemory() {
		if err := f.guardDisk(); err != nil {
			if errors.Is(err, errSymlink) {
				d, _ := f.Dir()
				return os.Remove(d) // the link, not what it points to
			}
			return err
		}
	}
	names, err := f.All()
	if err != nil {
		return err
	}
	for _, n := range names {
		if err := f.Remove(n); err != nil {
			return err
		}
	}
	if dir, err := f.Dir(); err == nil && f != Root {
		// The empty folders left. A virus scanner or indexer can hold one
		// open for a moment on Windows; an empty folder that stays is not a
		// failure, only files that remain are.
		if err := os.RemoveAll(dir); err != nil {
			if left, _ := f.All(); len(left) > 0 {
				return err
			}
		}
	}
	return nil
}

var (
	errSymlink = errors.New("folder is a symbolic link")
	errEscape  = errors.New("folder is outside the user's folder")
)

// guardDisk checks that the folder, on disk, is a real folder inside the
// user's folder (or is not there).
func (f Folder) guardDisk() error {
	dir, err := f.Dir()
	if err != nil {
		return nil // a web browser has no folders to escape to
	}
	if !fs.ValidPath(string(f)) || string(f) == "." {
		return errEscape
	}
	fi, err := os.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	case fi.Mode()&fs.ModeSymlink != 0:
		return errSymlink
	}
	root, err := diskDir()
	if err != nil {
		return err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	if rel, err := filepath.Rel(realRoot, realDir); err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errEscape
	}
	return nil
}
