//go:build js

package browser

import (
	"syscall/js"
	"testing"
)

// A lock without a clientId (or with one that isn't a string) is no
// client at all, never "<undefined>": an unknown client never counts as
// "still ours" after a restore.
func TestJSString(t *testing.T) {
	o := js.Global().Get("Object").New()
	o.Set("clientId", "4F0D36B2")
	o.Set("n", 7)
	o.Set("z", js.Null())
	for _, c := range []struct {
		key, want string
	}{
		{"clientId", "4F0D36B2"},
		{"missing", ""},
		{"z", ""},
		{"n", ""},
	} {
		if got := jsString(o.Get(c.key)); got != c.want {
			t.Errorf("%s: %q, want %q", c.key, got, c.want)
		}
	}
}
