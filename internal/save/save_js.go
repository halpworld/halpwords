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

// diskDir fails in a browser: there is no folder, only local storage.
func diskDir() (string, error) { return "", errors.New("no user folder in a web browser") }

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

// diskRead returns the contents of the named file.
func diskRead(name string) ([]byte, error) {
	v, err := storage("getItem", prefix+name)
	if err != nil {
		return nil, err
	}
	if v.IsNull() {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrNotExist}
	}
	return []byte(v.String()), nil
}

// diskWrite replaces the named file.
func diskWrite(name string, data []byte) error {
	_, err := storage("setItem", prefix+name, string(data))
	return err
}

// diskWritePrivate replaces the named file. Local storage belongs to the page,
// so it is as private as a browser allows.
func diskWritePrivate(name string, data []byte) error { return diskWrite(name, data) }

// diskRemove deletes the named file.
func diskRemove(name string) error {
	_, err := storage("removeItem", prefix+name)
	return err
}

// diskList returns the names of the files in the folder dir, sorted.
func diskList(dir string) ([]string, error) {
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

// diskAll returns the names of every file, in every folder, sorted.
func diskAll() ([]string, error) {
	ls, err := storage("valueOf")
	if err != nil {
		return nil, err
	}
	var names []string
	for i := 0; i < ls.Get("length").Int(); i++ {
		k, err := storage("key", i)
		if err != nil {
			return nil, err
		}
		if k.IsNull() {
			continue
		}
		if name, ok := strings.CutPrefix(k.String(), prefix); ok && name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}
