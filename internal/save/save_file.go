//go:build !js

package save

import (
	"os"
	"path"
	"path/filepath"
	"sort"
)

// Dir is where saves, settings and the user's own word lists live, e.g.
// ~/Library/Application Support/halpwords on macOS.
func Dir() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "halpwords"), nil
}

// Read returns the contents of the named file.
func Read(name string) ([]byte, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
}

// Write replaces the named file. It writes a temporary file first, so a
// crash part way through never leaves half a save.
func Write(name string, data []byte) error {
	root, err := Dir()
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

// Remove deletes the named file. Removing a file that is not there is not an
// error.
func Remove(name string) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, filepath.FromSlash(name))); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// List returns the names of the files in the folder dir, sorted. A folder
// that is not there is empty.
func List(dir string) ([]string, error) {
	root, err := Dir()
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
