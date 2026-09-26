//go:build js

package browser

import (
	"fmt"
	"syscall/js"
)

// InBrowser reports whether the game runs in a web browser.
const InBrowser = true

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

// Here is the web game's address, without its query or fragment: where
// the website sends the player back to.
func Here() string {
	var s string
	call(func() {
		loc := js.Global().Get("location")
		s = loc.Get("origin").String() + loc.Get("pathname").String()
	})
	return s
}

// open leaves the game for url, keeping the game in the history so the
// back button returns to it.
func open(url string) error {
	return call(func() { js.Global().Get("location").Call("assign", url) })
}
