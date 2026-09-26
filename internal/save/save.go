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
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
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
func (f Folder) Write(name string, data []byte) error {
	name = f.name(name)
	memMu.Lock()
	defer memMu.Unlock()
	if memory == nil {
		return diskWrite(name, data)
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

// List returns the names of the files in the folder dir, sorted. A folder
// that is not there is empty.
func (f Folder) List(dir string) ([]string, error) {
	dir = f.name(strings.TrimSuffix(dir, "/"))
	memMu.Lock()
	defer memMu.Unlock()
	if memory == nil {
		return diskList(dir)
	}
	var names []string
	for name := range memory {
		if path.Dir(name) == dir {
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

// RemoveAll deletes every file in the folder.
func (f Folder) RemoveAll() error {
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
		os.RemoveAll(dir) // the empty folders left
	}
	return nil
}
