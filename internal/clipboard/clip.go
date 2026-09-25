// Package clipboard reads text from the system clipboard, so a long API key can
// be pasted instead of typed. Ebitengine has no clipboard, so on the desktop
// it asks the system's own paste command, and in a browser it asks the
// page. Reading can be slow, so call it from a goroutine.
package clipboard

import "errors"

// ErrNoClipboard means the clipboard could not be read on this system.
var ErrNoClipboard = errors.New("the clipboard can't be read here")
