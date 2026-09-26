//go:build js

package link

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"syscall/js"
)

// jsSocket is the browser's WebSocket. The browser does the framing,
// masking and pings; a page can't send headers, which is why the socket
// takes a ticket.
type jsSocket struct {
	ws    js.Value
	funcs []js.Func
	max   int

	mu     sync.Mutex
	queue  [][]byte
	err    error         // set once the socket has closed
	signal chan struct{} // a message or the close is waiting
}

// dialSocket opens a WebSocket to u. The browser doesn't say why a
// handshake failed, so a refused ticket is just an error.
func dialSocket(ctx context.Context, u *url.URL, _ http.Header, max int) (socket, error) {
	ctor := js.Global().Get("WebSocket")
	if ctor.IsUndefined() {
		return nil, errors.New("link: this browser has no WebSocket")
	}
	s := &jsSocket{ws: ctor.New(u.String()), max: max, signal: make(chan struct{}, 1)}
	opened := make(chan struct{})
	var once sync.Once
	on := func(event string, fn func(js.Value)) {
		f := js.FuncOf(func(_ js.Value, args []js.Value) any {
			fn(args[0])
			return nil
		})
		s.funcs = append(s.funcs, f)
		s.ws.Call("addEventListener", event, f)
	}
	on("open", func(js.Value) { once.Do(func() { close(opened) }) })
	on("message", func(e js.Value) {
		data := e.Get("data")
		if data.Type() != js.TypeString {
			s.fail(errors.Join(errProtocol, errors.New("a binary message")))
			s.ws.Call("close", 1000)
			return
		}
		msg := []byte(data.String())
		s.mu.Lock()
		if len(msg) > s.max {
			s.mu.Unlock()
			s.fail(errTooBig)
			s.ws.Call("close", 1000)
			return
		}
		s.queue = append(s.queue, msg)
		s.mu.Unlock()
		s.wake()
	})
	on("close", func(e js.Value) {
		s.fail(&CloseError{Code: e.Get("code").Int(), Reason: e.Get("reason").String()})
		once.Do(func() { close(opened) })
	})
	select {
	case <-opened:
	case <-ctx.Done():
		s.Close(1000, "")
		return nil, ctx.Err()
	}
	s.mu.Lock()
	err := s.err
	s.mu.Unlock()
	if err != nil {
		s.release()
		return nil, errors.New("link: can't open a websocket to the server")
	}
	return s, nil
}

func (s *jsSocket) wake() {
	select {
	case s.signal <- struct{}{}:
	default:
	}
}

// fail records why the socket closed, once.
func (s *jsSocket) fail(err error) {
	s.mu.Lock()
	if s.err == nil {
		s.err = err
	}
	s.mu.Unlock()
	s.wake()
}

// release lets go of the event handlers.
func (s *jsSocket) release() {
	s.mu.Lock()
	funcs := s.funcs
	s.funcs = nil
	s.mu.Unlock()
	for _, f := range funcs {
		f.Release()
	}
}

// Read returns the next text message, or why the socket closed.
func (s *jsSocket) Read() ([]byte, error) {
	for {
		s.mu.Lock()
		if len(s.queue) > 0 {
			msg := s.queue[0]
			s.queue = s.queue[1:]
			s.mu.Unlock()
			return msg, nil
		}
		err := s.err
		s.mu.Unlock()
		if err != nil {
			s.release()
			return nil, err
		}
		<-s.signal
	}
}

// Write sends a text message.
func (s *jsSocket) Write(msg []byte) error {
	s.mu.Lock()
	err := s.err
	s.mu.Unlock()
	if err != nil {
		return err
	}
	s.ws.Call("send", string(msg))
	return nil
}

// Close closes the socket. A page may only close with 1000 or 3000-4999,
// so the code is always 1000.
func (s *jsSocket) Close(int, string) error {
	s.ws.Call("close", 1000)
	s.fail(errClosed)
	return nil
}
