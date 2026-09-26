//go:build !js

package save

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
)

// diskDir is where saves, settings and the user's own word lists live, e.g.
// ~/Library/Application Support/halpwords on macOS.
func diskDir() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "halpwords"), nil
}

// diskRead returns the contents of the named file.
func diskRead(name string) ([]byte, error) {
	dir, err := diskDir()
	if err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
}

// diskWrite replaces the named file. It writes a temporary file first, so a
// crash part way through never leaves half a save.
func diskWrite(name string, data []byte) error {
	root, err := diskDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(root, filepath.FromSlash(path.Dir(name)))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, path.Base(name)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, path.Base(name)))
}

// diskWritePrivate replaces the named file with one only the user can read, for
// secrets such as API keys.
func diskWritePrivate(name string, data []byte) error {
	if err := diskWrite(name, data); err != nil {
		return err
	}
	dir, err := diskDir()
	if err != nil {
		return err
	}
	return os.Chmod(filepath.Join(dir, filepath.FromSlash(name)), 0o600)
}

// diskRemove deletes the named file. Removing a file that is not there is not an
// error.
func diskRemove(name string) error {
	dir, err := diskDir()
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, filepath.FromSlash(name))); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// diskList returns the names of the files in the folder dir, sorted. A folder
// that is not there is empty.
func diskList(dir string) ([]string, error) {
	root, err := diskDir()
	if err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(dir)))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range ents {
		if e.Type().IsRegular() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// diskAll returns the names of every file, in every folder, sorted, with
// forward slashes. A missing user folder is empty.
func diskAll() ([]string, error) {
	root, err := diskDir()
	if err != nil {
		return nil, err
	}
	var names []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == root && os.IsNotExist(err) {
				return fs.SkipAll
			}
			return err
		}
		if d.Type().IsRegular() {
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			names = append(names, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}
