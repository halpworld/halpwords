//go:build js

package save

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"syscall/js"
)

const prefix = "halpwords/"

// Dir fails in a browser: there is no folder, only local storage.
func Dir() (string, error) { return "", errors.New("no user folder in a web browser") }

// storage calls a local storage method. Browsers throw when storage is off or
// full, which syscall/js turns into a panic.
func storage(method string, args ...any) (v js.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("local storage: %v", r)
		}
	}()
	ls := js.Global().Get("localStorage")
	if ls.IsUndefined() || ls.IsNull() {
		return js.Null(), errors.New("local storage is not available")
	}
	return ls.Call(method, args...), nil
}

// Read returns the contents of the named file.
func Read(name string) ([]byte, error) {
	v, err := storage("getItem", prefix+name)
	if err != nil {
		return nil, err
	}
	if v.IsNull() {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrNotExist}
	}
	return []byte(v.String()), nil
}

// Write replaces the named file.
func Write(name string, data []byte) error {
	_, err := storage("setItem", prefix+name, string(data))
	return err
}

// Remove deletes the named file.
func Remove(name string) error {
	_, err := storage("removeItem", prefix+name)
	return err
}

// List returns the names of the files in the folder dir, sorted.
func List(dir string) ([]string, error) {
	ls, err := storage("valueOf") // the storage itself, or an error when it is off
	if err != nil {
		return nil, err
	}
	want := prefix + strings.TrimSuffix(dir, "/") + "/"
	var names []string
	for i := 0; i < ls.Get("length").Int(); i++ {
		k, err := storage("key", i)
		if err != nil {
			return nil, err
		}
		if k.IsNull() {
			continue
		}
		if name, ok := strings.CutPrefix(k.String(), want); ok && name != "" && !strings.Contains(name, "/") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}
