//go:build js

package move

import (
	"fmt"
	"syscall/js"
)

// call runs f, turning a JavaScript exception (a panic in syscall/js) into
// an error.
func call(f func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("browser: %v", r)
		}
	}()
	f()
	return nil
}

// Fragment returns the page's URL fragment, without the "#".
func Fragment() string {
	var s string
	call(func() {
		h := js.Global().Get("location").Get("hash")
		if h.Type() == js.TypeString {
			s = h.String()
		}
	})
	if len(s) > 0 && s[0] == '#' {
		s = s[1:]
	}
	return s
}

// ClearFragment takes the fragment off the address without reloading the
// page or adding to the history, so a reload doesn't import again.
func ClearFragment() error {
	return call(func() {
		loc := js.Global().Get("location")
		url := loc.Get("pathname").String() + loc.Get("search").String()
		js.Global().Get("history").Call("replaceState", js.Null(), "", url)
	})
}

// Go leaves the game for url, replacing this page in the history.
func Go(url string) error {
	return call(func() { js.Global().Get("location").Call("replace", url) })
}
