package link

import (
	"bufio"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"
)

// The part of the WebSocket protocol (RFC 6455) the desktop game needs to
// talk to rooms (/api/v1/play): text messages, ping and pong, close, and
// masking what the client sends. The web build uses the browser's
// WebSocket instead (ws_js.go). No extensions and no subprotocols.

// Frame opcodes (RFC 6455 §5.2).
const (
	opContinuation = 0x0
	opText         = 0x1
	opBinary       = 0x2
	opClose        = 0x8
	opPing         = 0x9
	opPong         = 0xA
)

// maxControl is the most a control frame may carry (RFC 6455 §5.5).
const maxControl = 125

// CloseError is the server closing the connection, with its close code
// and reason (RFC 6455 §7.4), such as 1008 "bad-message" or 1012
// "shutdown".
type CloseError struct {
	Code   int
	Reason string
}

func (e *CloseError) Error() string {
	return fmt.Sprintf("the server closed the connection (%d %s)", e.Code, e.Reason)
}

// errProtocol is a frame that breaks RFC 6455; the connection is closed
// with 1002.
var errProtocol = errors.New("link: websocket protocol error")

// errTooBig is a message over the limit; the connection is closed with
// 1009.
var errTooBig = errors.New("link: websocket message too big")

// errClosed is using a socket the game closed.
var errClosed = errors.New("link: the websocket is closed")

// socket is a WebSocket to the server, carrying text messages: wsConn on
// the desktop, the browser's WebSocket on the web.
type socket interface {
	// Read returns the next text message; a close from the server is a
	// *CloseError.
	Read() ([]byte, error)
	// Write sends a text message.
	Write(msg []byte) error
	// Close closes the connection with a close code and reason.
	Close(code int, reason string) error
}

// frame is one frame as read.
type frame struct {
	fin     bool
	op      byte
	payload []byte
}

// readFrame reads one frame from the server. Server frames are never
// masked, and no extension is agreed, so the RSV bits are 0. A frame
// longer than max is refused before its payload is read.
func readFrame(r *bufio.Reader, max int) (frame, error) {
	var f frame
	var h [2]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return f, err
	}
	f.fin = h[0]&0x80 != 0
	if h[0]&0x70 != 0 {
		return f, fmt.Errorf("%w: reserved bits set", errProtocol)
	}
	f.op = h[0] & 0x0F
	if h[1]&0x80 != 0 {
		return f, fmt.Errorf("%w: a masked frame from the server", errProtocol)
	}
	var n uint64
	switch l := h[1] & 0x7F; l {
	case 126:
		var b [2]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return f, err
		}
		n = uint64(binary.BigEndian.Uint16(b[:]))
		if n < 126 {
			return f, fmt.Errorf("%w: length not in its shortest form", errProtocol)
		}
	case 127:
		var b [8]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return f, err
		}
		n = binary.BigEndian.Uint64(b[:])
		if n>>63 != 0 {
			return f, fmt.Errorf("%w: length with the top bit set", errProtocol)
		}
		if n <= 0xFFFF {
			return f, fmt.Errorf("%w: length not in its shortest form", errProtocol)
		}
	default:
		n = uint64(l)
	}
	switch f.op {
	case opContinuation, opText, opBinary:
	case opClose, opPing, opPong:
		if !f.fin || n > maxControl {
			return f, fmt.Errorf("%w: a fragmented or long control frame", errProtocol)
		}
	default:
		return f, fmt.Errorf("%w: unknown opcode %d", errProtocol, f.op)
	}
	if n > uint64(max) {
		return f, errTooBig
	}
	f.payload = make([]byte, n)
	if _, err := io.ReadFull(r, f.payload); err != nil {
		return f, err
	}
	return f, nil
}

// readMessage reads frames until a whole data message, answering control
// frames through control as they come (between the fragments of a
// message too). It returns the message's opcode (text or binary) and its
// payload, at most max bytes; a text message must be UTF-8. A close from
// the server is a *CloseError.
func readMessage(r *bufio.Reader, max int, control func(op byte, payload []byte) error) (byte, []byte, error) {
	var (
		op  byte
		msg []byte
		mid bool // inside a fragmented message
	)
	for {
		f, err := readFrame(r, max-len(msg))
		if err != nil {
			return 0, nil, err
		}
		switch f.op {
		case opClose:
			ce := &CloseError{Code: 1005}
			switch {
			case len(f.payload) == 1:
				return 0, nil, fmt.Errorf("%w: a one-byte close", errProtocol)
			case len(f.payload) >= 2:
				ce.Code = int(binary.BigEndian.Uint16(f.payload))
				if !utf8.Valid(f.payload[2:]) {
					return 0, nil, fmt.Errorf("%w: a close reason that isn't UTF-8", errProtocol)
				}
				ce.Reason = string(f.payload[2:])
			}
			if control != nil {
				control(opClose, f.payload)
			}
			return 0, nil, ce
		case opPing, opPong:
			if control != nil {
				if err := control(f.op, f.payload); err != nil {
					return 0, nil, err
				}
			}
			continue
		case opContinuation:
			if !mid {
				return 0, nil, fmt.Errorf("%w: a continuation with no message", errProtocol)
			}
		default: // text or binary
			if mid {
				return 0, nil, fmt.Errorf("%w: a new message inside a fragmented one", errProtocol)
			}
			op, mid = f.op, true
		}
		msg = append(msg, f.payload...)
		if f.fin {
			if op == opText && !utf8.Valid(msg) {
				return 0, nil, fmt.Errorf("%w: a text message that isn't UTF-8", errProtocol)
			}
			return op, msg, nil
		}
	}
}

// appendFrame appends one final, masked client frame (RFC 6455 §5.3).
func appendFrame(dst []byte, op byte, payload []byte) []byte {
	dst = append(dst, 0x80|op)
	n := len(payload)
	switch {
	case n <= 125:
		dst = append(dst, 0x80|byte(n))
	case n <= 0xFFFF:
		dst = append(dst, 0x80|126, byte(n>>8), byte(n))
	default:
		dst = append(dst, 0x80|127)
		dst = binary.BigEndian.AppendUint64(dst, uint64(n))
	}
	var key [4]byte
	rand.Read(key[:])
	dst = append(dst, key[:]...)
	start := len(dst)
	dst = append(dst, payload...)
	for i := range payload {
		dst[start+i] ^= key[i%4]
	}
	return dst
}

// closePayload is a close frame's payload: the code and a short reason.
func closePayload(code int, reason string) []byte {
	if len(reason) > maxControl-2 {
		reason = reason[:maxControl-2]
	}
	b := binary.BigEndian.AppendUint16(nil, uint16(code))
	return append(b, reason...)
}
