//go:build !js

package link

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// wsGUID is the key the server's Sec-WebSocket-Accept is made with (RFC
// 6455 §1.3).
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// wsConn is a WebSocket connection on the desktop, on a plain TCP or TLS
// connection. Read answers pings itself.
type wsConn struct {
	conn net.Conn
	br   *bufio.Reader
	max  int

	wmu    sync.Mutex // one frame at a time
	closed bool
}

// dialSocket opens a WebSocket to u (ws:// or wss://), sending hdr with
// the handshake. An answer other than 101 is an *Error, like any other
// answer of the server.
func dialSocket(ctx context.Context, u *url.URL, hdr http.Header, max int) (socket, error) {
	host := u.Host
	var conn net.Conn
	var err error
	switch u.Scheme {
	case "ws":
		if u.Port() == "" {
			host = net.JoinHostPort(u.Hostname(), "80")
		}
		var d net.Dialer
		conn, err = d.DialContext(ctx, "tcp", host)
	case "wss":
		if u.Port() == "" {
			host = net.JoinHostPort(u.Hostname(), "443")
		}
		d := tls.Dialer{Config: &tls.Config{ServerName: u.Hostname(), MinVersion: tls.VersionTLS12}}
		conn, err = d.DialContext(ctx, "tcp", host)
	default:
		return nil, fmt.Errorf("link: can't open a websocket to %s", u.Scheme)
	}
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			conn.Close()
		}
	}()
	if dl, has := ctx.Deadline(); has {
		conn.SetDeadline(dl)
	}
	var k [16]byte
	rand.Read(k[:])
	key := base64.StdEncoding.EncodeToString(k[:])
	req := &http.Request{
		Method: http.MethodGet,
		URL:    u,
		Host:   u.Host,
		Header: http.Header{},
	}
	for name, v := range hdr {
		req.Header[name] = v
	}
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Key", key)
	req.Header.Set("Sec-WebSocket-Version", "13")
	// req.Write sends the path and query of URL; ws and wss are only
	// the scheme.
	if err := req.Write(conn); err != nil {
		return nil, err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
		return nil, errorFrom(resp.StatusCode, resp.Header, data)
	}
	if !strings.EqualFold(resp.Header.Get("Upgrade"), "websocket") ||
		!headerHas(resp.Header, "Connection", "upgrade") ||
		resp.Header.Get("Sec-WebSocket-Accept") != acceptKey(key) {
		return nil, errors.New("link: the server didn't open a websocket")
	}
	if resp.Header.Get("Sec-WebSocket-Extensions") != "" || resp.Header.Get("Sec-WebSocket-Protocol") != "" {
		return nil, errors.New("link: the server chose a websocket extension the game didn't ask for")
	}
	conn.SetDeadline(time.Time{})
	ok = true
	return &wsConn{conn: conn, br: br, max: max}, nil
}

// acceptKey is the Sec-WebSocket-Accept for a Sec-WebSocket-Key.
func acceptKey(key string) string {
	h := sha1.Sum([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(h[:])
}

// headerHas reports whether a comma-separated header has a token.
func headerHas(h http.Header, name, token string) bool {
	for _, v := range h.Values(name) {
		for _, t := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(t), token) {
				return true
			}
		}
	}
	return false
}

// Read returns the next text message. A protocol error, a binary message
// or one over the limit closes the connection.
func (w *wsConn) Read() ([]byte, error) {
	op, msg, err := readMessage(w.br, w.max, func(op byte, payload []byte) error {
		switch op {
		case opPing:
			return w.writeFrame(opPong, payload)
		case opClose:
			// Answer the close with the same code (RFC 6455 §5.5.1).
			code := 1000
			if len(payload) >= 2 {
				code = int(payload[0])<<8 | int(payload[1])
			}
			w.Close(code, "")
		}
		return nil
	})
	switch {
	case errors.Is(err, errProtocol):
		w.Close(1002, "")
	case errors.Is(err, errTooBig):
		w.Close(1009, "")
	case err == nil && op != opText:
		w.Close(1003, "text-only")
		return nil, fmt.Errorf("%w: a binary message", errProtocol)
	}
	return msg, err
}

// Write sends a text message.
func (w *wsConn) Write(msg []byte) error {
	return w.writeFrame(opText, msg)
}

func (w *wsConn) writeFrame(op byte, payload []byte) error {
	w.wmu.Lock()
	defer w.wmu.Unlock()
	if w.closed {
		return errClosed
	}
	w.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := w.conn.Write(appendFrame(nil, op, payload))
	return err
}

// Close sends a close frame and closes the connection; it doesn't wait
// for the server's answer.
func (w *wsConn) Close(code int, reason string) error {
	w.wmu.Lock()
	if !w.closed {
		w.conn.SetWriteDeadline(time.Now().Add(time.Second))
		w.conn.Write(appendFrame(nil, opClose, closePayload(code, reason)))
		w.closed = true
	}
	w.wmu.Unlock()
	return w.conn.Close()
}
