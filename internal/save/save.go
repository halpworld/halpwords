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
	"path"
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

// Dir is where saves, settings and the user's own word lists live, e.g.
// ~/Library/Application Support/halpwords on macOS. It fails in a web
// browser, which has only local storage, and while files are kept in
// memory.
func Dir() (string, error) {
	if InMemory() {
		return "", errNoDir
	}
	return diskDir()
}

// Read returns the contents of the named file.
func Read(name string) ([]byte, error) {
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
func Write(name string, data []byte) error {
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
func WritePrivate(name string, data []byte) error {
	if InMemory() {
		return Write(name, data)
	}
	return diskWritePrivate(name, data)
}

// Remove deletes the named file. Removing a file that is not there is not
// an error.
func Remove(name string) error {
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
func List(dir string) ([]string, error) {
	memMu.Lock()
	defer memMu.Unlock()
	if memory == nil {
		return diskList(dir)
	}
	dir = strings.TrimSuffix(dir, "/")
	var names []string
	for name := range memory {
		if path.Dir(name) == dir {
			names = append(names, path.Base(name))
		}
	}
	sort.Strings(names)
	return names, nil
}
