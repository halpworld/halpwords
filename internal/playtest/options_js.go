//go:build js

package playtest

import (
	"net/http"
	"syscall/js"
)

// browserOptions asks the browser's fetch to stay on the page's origin,
// send no cookies, and refuse redirects. Go's WebAssembly HTTP client
// turns these headers into fetch options and doesn't send them.
func browserOptions(req *http.Request) {
	req.Header.Set("js.fetch:mode", "same-origin")
	req.Header.Set("js.fetch:credentials", "omit")
	req.Header.Set("js.fetch:redirect", "error")
}

// Page returns the web game's own address.
func Page() string {
	loc := js.Global().Get("location")
	if loc.IsUndefined() || loc.IsNull() {
		return ""
	}
	return loc.Get("href").String()
}
