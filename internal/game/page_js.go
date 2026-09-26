//go:build js

package game

import (
	"syscall/js"

	"github.com/halpworld/halpwords/internal/link"
)

// watchPage saves the link's queue when the page is hidden or closed, as a
// web game never gets to quit, and syncs when it is hidden so a closed tab
// loses as little as it can. The handlers stay for the life of the page,
// and act on the link of the learner playing (current).
func watchPage(current func() *link.Client) {
	doc := js.Global().Get("document")
	if doc.IsUndefined() {
		return
	}
	hidden := js.FuncOf(func(js.Value, []js.Value) any {
		if doc.Get("visibilityState").String() == "hidden" {
			l := current()
			go func() {
				l.Save()
				l.SyncNow()
			}()
		}
		return nil
	})
	doc.Call("addEventListener", "visibilitychange", hidden)
	gone := js.FuncOf(func(js.Value, []js.Value) any {
		current().TrySave() // now, and without waiting: the page is going away
		return nil
	})
	js.Global().Call("addEventListener", "pagehide", gone)
}
