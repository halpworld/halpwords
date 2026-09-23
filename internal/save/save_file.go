//go:build !js

package save

import (
	"os"
	"path/filepath"
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
	return os.ReadFile(filepath.Join(dir, name))
}

// Write replaces the named file. It writes a temporary file first, so a
// crash part way through never leaves half a save.
func Write(name string, data []byte) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, name+".*.tmp")
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
	return os.Rename(tmp.Name(), filepath.Join(dir, name))
}

// Remove deletes the named file. Removing a file that is not there is not an
// error.
func Remove(name string) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
