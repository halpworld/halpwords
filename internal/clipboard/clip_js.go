//go:build js

package clipboard

import (
	"fmt"
	"syscall/js"
)

// Read returns the text on the clipboard. The browser may ask the player
// for permission first.
func Read() (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", ErrNoClipboard, r)
		}
	}()
	cb := js.Global().Get("navigator").Get("clipboard")
	if cb.IsUndefined() || cb.Get("readText").IsUndefined() {
		return "", ErrNoClipboard
	}
	type result struct {
		text string
		err  error
	}
	ch := make(chan result, 1)
	var ok, fail js.Func
	ok = js.FuncOf(func(_ js.Value, args []js.Value) any {
		ch <- result{text: args[0].String()}
		return nil
	})
	fail = js.FuncOf(func(_ js.Value, args []js.Value) any {
		ch <- result{err: fmt.Errorf("%w: %s", ErrNoClipboard, args[0].Call("toString").String())}
		return nil
	})
	defer ok.Release()
	defer fail.Release()
	cb.Call("readText").Call("then", ok, fail)
	r := <-ch
	return r.text, r.err
}
