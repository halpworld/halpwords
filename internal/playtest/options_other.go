//go:build !js

package playtest

import "net/http"

// browserOptions does nothing outside a web browser.
func browserOptions(*http.Request) {}

// Page returns "": only the web game has a page address.
func Page() string { return "" }
