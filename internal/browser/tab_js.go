//go:build js

package browser

import (
	"sync"
	"syscall/js"
	"time"
)

// tabLock is the Web Lock the tab that plays holds, for the page's life.
const tabLock = "halpwords-saves"

// tabWait is how long HoldTab waits for the browser's first answer
// before it says "not yet".
const tabWait = 2 * time.Second

// tab is this page's hold of tabLock.
var tab struct {
	mu sync.Mutex
	// asked: the first request was made. queued: this page waits in
	// the browser's queue for the lock (a request without ifAvailable),
	// so it gets it as soon as the tab that holds it closes, even while
	// this tab is in the background and the game isn't running.
	asked, queued bool
	// held: the browser gave this page the lock, possibly after a
	// HoldTab that stopped waiting for it.
	held bool
	// client is this page's clientId, as navigator.locks.query() names
	// the holder; "" until it is known.
	client string
	// guarded: the pageshow check is installed.
	guarded bool
}

// HoldTab reports whether this tab holds the lock that lets one tab of
// the web game read and write the saves (halpwords#111). Two tabs share
// one local storage: both writing it would fork a suspended run and write
// one tab's old link tokens over the other's. It never waits for another
// tab.
//
// The first call asks for the lock if no tab holds it, and waits up to
// tabWait for the answer. If another tab holds it, this page joins the
// browser's queue for it once, and gets it when that tab closes; later
// calls only say whether it has. If the browser hasn't answered in time
// (a slow computer), it is "not yet": a late answer counts at the next
// call. The tab that gets the lock keeps it until the page closes.
//
// Only a browser with no usable Web Locks (older ones, or a page that
// isn't a secure context) gets true without a lock: the game then plays
// unguarded, as before, rather than not at all.
func HoldTab() bool {
	locks, ok := webLocks()
	if !ok {
		return true
	}
	tab.mu.Lock()
	if tab.held || tab.asked {
		held := tab.held
		tab.mu.Unlock()
		return held
	}
	tab.asked = true
	tab.mu.Unlock()
	first := make(chan bool, 1)
	opts := js.Global().Get("Object").New()
	opts.Set("ifAvailable", true)
	if !request(locks, opts, func(got bool) {
		if !got {
			queue(locks) // held elsewhere: wait for it in the browser
		}
		first <- got
	}) {
		return true // Web Locks that can't be used: unguarded
	}
	select {
	case got := <-first:
		return got
	case <-time.After(tabWait):
		return false // not yet: the answer, when it comes, counts later
	}
}

// queue joins the browser's queue for the lock, once.
func queue(locks js.Value) {
	tab.mu.Lock()
	if tab.queued || tab.held {
		tab.mu.Unlock()
		return
	}
	tab.queued = true
	tab.mu.Unlock()
	request(locks, js.Global().Get("Object").New(), func(bool) {})
}

// request asks for the lock with opts, and calls done with whether this
// page got it. A page that gets it keeps it for its life, learns its
// clientId and checks it again after a back-forward cache restore. It
// reports false if the browser refused the request outright.
func request(locks, opts js.Value, done func(bool)) bool {
	var cb js.Func
	cb = js.FuncOf(func(_ js.Value, args []js.Value) any {
		got := len(args) > 0 && !args[0].IsNull() && !args[0].IsUndefined()
		if !got {
			cb.Release()
			done(false)
			return nil // the request ends at once
		}
		tab.mu.Lock()
		tab.held = true
		tab.mu.Unlock()
		learnClient(locks)
		guardRestore(locks)
		done(true)
		// A promise that never settles: the lock is this page's for
		// as long as it lives. cb stays referenced for that time.
		return js.Global().Get("Promise").New(js.FuncOf(func(js.Value, []js.Value) any { return nil }))
	})
	if call(func() { locks.Call("request", tabLock, opts, cb) }) != nil {
		cb.Release()
		return false
	}
	return true
}

// webLocks is navigator.locks, and whether the browser has it.
func webLocks() (js.Value, bool) {
	var locks js.Value
	if call(func() { locks = js.Global().Get("navigator").Get("locks") }) != nil || locks.IsUndefined() || locks.IsNull() {
		return js.Value{}, false
	}
	return locks, true
}

// learnClient finds this page's clientId: right after this page got the
// lock, it is the lock's only holder.
func learnClient(locks js.Value) {
	onQuery(locks, func(held []heldLock) {
		for _, l := range held {
			if l.name == tabLock {
				tab.mu.Lock()
				tab.client = l.client
				tab.mu.Unlock()
				return
			}
		}
	})
}

// guardRestore makes a page that comes back from the back-forward cache
// check that it still holds the lock: a browser may have let it go while
// the page slept, and another tab may hold it now. If it doesn't, the
// page reloads at once, before the game writes anything, and so waits
// for the other tab like any second tab (#111, #105). The web page
// reloads after such a restore anyway (#103); this doesn't rely on that,
// nor on a browser keeping pages with a lock out of that cache.
func guardRestore(locks js.Value) {
	tab.mu.Lock()
	defer tab.mu.Unlock()
	if tab.guarded {
		return
	}
	tab.guarded = true
	js.Global().Call("addEventListener", "pageshow", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 0 || !args[0].Get("persisted").Truthy() {
			return nil
		}
		checkHold(locks)
		return nil
	}))
}

// checkHold reloads the page if it no longer holds the lock it took (or
// can't tell, its clientId being unknown).
func checkHold(locks js.Value) {
	tab.mu.Lock()
	client := tab.client
	tab.mu.Unlock()
	onQuery(locks, func(hs []heldLock) {
		for _, l := range hs {
			if l.name == tabLock && client != "" && l.client == client {
				return // still ours
			}
		}
		tab.mu.Lock()
		tab.held = false
		tab.mu.Unlock()
		call(func() { js.Global().Get("location").Call("reload") })
	})
}

// jsString is v when it is a string, or "": syscall/js turns undefined
// and null into "<undefined>" and "<null>", which would make a browser
// that doesn't give LockInfo a clientId match every holder with every
// other ("still ours" after a restore, when another tab has the lock).
func jsString(v js.Value) string {
	if v.Type() != js.TypeString {
		return ""
	}
	return v.String()
}

// heldLock is one held lock, as navigator.locks.query() lists it.
type heldLock struct{ name, client string }

// onQuery calls f with the locks held now, once the browser answers.
func onQuery(locks js.Value, f func([]heldLock)) {
	var then js.Func
	then = js.FuncOf(func(_ js.Value, args []js.Value) any {
		defer then.Release()
		if len(args) == 0 {
			return nil
		}
		hs := args[0].Get("held")
		var out []heldLock
		for i := 0; i < hs.Length(); i++ {
			l := hs.Index(i)
			out = append(out, heldLock{name: jsString(l.Get("name")), client: jsString(l.Get("clientId"))})
		}
		f(out)
		return nil
	})
	if call(func() { locks.Call("query").Call("then", then) }) != nil {
		then.Release()
	}
}
