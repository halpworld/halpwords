//go:build js

package browser

import (
	"syscall/js"
	"time"
)

// tabLock is the Web Lock the tab that plays holds, for the page's life.
const tabLock = "halpwords-saves"

// tabWait is how long HoldTab waits for the browser's answer.
const tabWait = 2 * time.Second

// HoldTab tries, without waiting for another tab, to take the lock that
// lets one tab of the web game read and write the saves (halpwords#111).
// Two tabs share one local storage: both writing it would fork a
// suspended run and write one tab's old link tokens over the other's.
// The tab that gets it keeps it until the page is closed (or, when the
// browser keeps pages with a lock out of its back-forward cache, gone).
// It reports true when this tab holds it, and also when the browser has
// no Web Locks (older browsers) or doesn't answer in time: the game
// then plays as before, unguarded, rather than not at all.
func HoldTab() bool {
	var locks js.Value
	if call(func() { locks = js.Global().Get("navigator").Get("locks") }) != nil || locks.IsUndefined() || locks.IsNull() {
		return true
	}
	got := make(chan bool, 1)
	var cb js.Func
	cb = js.FuncOf(func(_ js.Value, args []js.Value) any {
		held := len(args) > 0 && !args[0].IsNull() && !args[0].IsUndefined()
		got <- held
		if !held {
			cb.Release()
			return nil // the request ends at once
		}
		// A promise that never settles: the lock is this page's for
		// as long as it lives. cb stays referenced for that time.
		return js.Global().Get("Promise").New(js.FuncOf(func(js.Value, []js.Value) any { return nil }))
	})
	opts := js.Global().Get("Object").New()
	opts.Set("ifAvailable", true)
	if call(func() { locks.Call("request", tabLock, opts, cb) }) != nil {
		cb.Release()
		return true
	}
	select {
	case held := <-got:
		return held
	case <-time.After(tabWait):
		return true
	}
}
