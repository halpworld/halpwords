//go:build !js

package move

import "errors"

// Fragment returns the page's URL fragment. Outside a browser there is none.
func Fragment() string { return "" }

// ClearFragment does nothing outside a browser.
func ClearFragment() error { return nil }

// Go fails outside a browser.
func Go(url string) error { return errors.New("not in a web browser") }
