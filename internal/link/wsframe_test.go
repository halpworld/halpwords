package link

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
)

// serverFrame is a frame as a server sends it: not masked.
func serverFrame(fin bool, op byte, payload []byte) []byte {
	b := []byte{op}
	if fin {
		b[0] |= 0x80
	}
	switch n := len(payload); {
	case n <= 125:
		b = append(b, byte(n))
	case n <= 0xFFFF:
		b = append(b, 126, byte(n>>8), byte(n))
	default:
		b = append(b, 127)
		b = binary.BigEndian.AppendUint64(b, uint64(n))
	}
	return append(b, payload...)
}

// readClientFrame reads a frame as the game sends it, checking it is
// final and masked, and unmasks it.
func readClientFrame(r *bufio.Reader) (byte, []byte, error) {
	var h [2]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return 0, nil, err
	}
	if h[0]&0x80 == 0 || h[0]&0x70 != 0 || h[1]&0x80 == 0 {
		return 0, nil, errors.New("not a final, masked frame")
	}
	n := uint64(h[1] & 0x7F)
	switch n {
	case 126:
		var b [2]byte
		io.ReadFull(r, b[:])
		n = uint64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		io.ReadFull(r, b[:])
		n = binary.BigEndian.Uint64(b[:])
	}
	var key [4]byte
	if _, err := io.ReadFull(r, key[:]); err != nil {
		return 0, nil, err
	}
	p := make([]byte, n)
	if _, err := io.ReadFull(r, p); err != nil {
		return 0, nil, err
	}
	for i := range p {
		p[i] ^= key[i%4]
	}
	return h[0] & 0x0F, p, nil
}

func reader(b ...[]byte) *bufio.Reader {
	return bufio.NewReader(bytes.NewReader(bytes.Join(b, nil)))
}

func TestFramesRoundTrip(t *testing.T) {
	for _, n := range []int{0, 1, 125, 126, 1000, 0xFFFF, 0x10000} {
		payload := bytes.Repeat([]byte("a"), n)
		f := appendFrame(nil, opText, payload)
		op, got, err := readClientFrame(bufio.NewReader(bytes.NewReader(f)))
		if err != nil || op != opText || !bytes.Equal(got, payload) {
			t.Fatalf("%d bytes: op %d, %d bytes, %v", n, op, len(got), err)
		}
		if n > 4 && bytes.Contains(f, payload[:4]) && bytes.Count(f, []byte("a")) >= n {
			t.Fatalf("%d bytes: not masked", n)
		}
		op, got, err = readMessage(reader(serverFrame(true, opText, payload)), 1<<20, nil)
		if err != nil || op != opText || !bytes.Equal(got, payload) {
			t.Fatalf("%d bytes from the server: op %d, %d bytes, %v", n, op, len(got), err)
		}
	}
}

func TestReadMessageFragmentsAndControl(t *testing.T) {
	var pings []string
	control := func(op byte, p []byte) error {
		if op == opPing {
			pings = append(pings, string(p))
		}
		return nil
	}
	op, msg, err := readMessage(reader(
		serverFrame(false, opText, []byte(`{"t":`)),
		serverFrame(true, opPing, []byte("hi")),
		serverFrame(false, opContinuation, []byte(`"pong"`)),
		serverFrame(true, opContinuation, []byte(`}`)),
	), 512, control)
	if err != nil || op != opText || string(msg) != `{"t":"pong"}` || strings.Join(pings, ",") != "hi" {
		t.Fatalf("%d %q %v, pings %v", op, msg, err, pings)
	}

	_, _, err = readMessage(reader(serverFrame(true, opClose, append([]byte{0x03, 0xF4}, "shutdown"...))), 512, nil)
	var ce *CloseError
	if !errors.As(err, &ce) || ce.Code != 1012 || ce.Reason != "shutdown" {
		t.Fatalf("close: %v", err)
	}
	_, _, err = readMessage(reader(serverFrame(true, opClose, nil)), 512, nil)
	if !errors.As(err, &ce) || ce.Code != 1005 {
		t.Fatalf("empty close: %v", err)
	}
}

func TestReadMessageRefuses(t *testing.T) {
	masked := serverFrame(true, opText, []byte("hi"))
	masked[1] |= 0x80
	for name, tc := range map[string]struct {
		in   []byte
		want error
	}{
		"reserved bits":          {append([]byte{0xC1, 0}, nil...), errProtocol},
		"masked":                 {masked, errProtocol},
		"unknown opcode":         {serverFrame(true, 0x3, nil), errProtocol},
		"fragmented ping":        {serverFrame(false, opPing, nil), errProtocol},
		"long ping":              {serverFrame(true, opPing, make([]byte, 126)), errProtocol},
		"lone continuation":      {serverFrame(true, opContinuation, []byte("x")), errProtocol},
		"new message in another": {append(serverFrame(false, opText, []byte("a")), serverFrame(true, opText, []byte("b"))...), errProtocol},
		"not UTF-8":              {serverFrame(true, opText, []byte{0xff, 0xfe}), errProtocol},
		"one-byte close":         {serverFrame(true, opClose, []byte{3}), errProtocol},
		"long form for a short":  {[]byte{0x81, 126, 0, 5, 'h', 'e', 'l', 'l', 'o'}, errProtocol},
		"top bit of a length":    {[]byte{0x81, 127, 0x80, 0, 0, 0, 0, 0, 0, 0}, errProtocol},
		"too big":                {serverFrame(true, opText, make([]byte, 513)), errTooBig},
		"too big in pieces": {append(serverFrame(false, opText, make([]byte, 300)),
			serverFrame(true, opContinuation, make([]byte, 300))...), errTooBig},
		// A huge length is refused before anything is allocated.
		"huge": {[]byte{0x81, 127, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, errTooBig},
	} {
		_, _, err := readMessage(reader(tc.in), 512, nil)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: %v, want %v", name, err, tc.want)
		}
	}
	if _, _, err := readMessage(reader(serverFrame(true, opText, []byte("abc"))[:3]), 512, nil); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("cut short: %v", err)
	}
}

// FuzzReadMessage: whatever the server sends, reading neither panics nor
// returns more than the limit, and a message it returns is whole UTF-8
// text or binary.
func FuzzReadMessage(f *testing.F) {
	f.Add(serverFrame(true, opText, []byte(`{"t":"hello"}`)))
	f.Add(append(serverFrame(false, opText, []byte("a")), serverFrame(true, opContinuation, []byte("b"))...))
	f.Add(serverFrame(true, opPing, []byte("x")))
	f.Add(serverFrame(true, opClose, []byte{3, 0xf4}))
	f.Add([]byte{0x81, 127, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	f.Add([]byte{0x81, 126, 0x01, 0x00})
	f.Fuzz(func(t *testing.T, data []byte) {
		r := reader(data)
		for range 8 {
			op, msg, err := readMessage(r, 256, func(byte, []byte) error { return nil })
			if err != nil {
				return
			}
			if len(msg) > 256 || (op != opText && op != opBinary) {
				t.Fatalf("op %d, %d bytes", op, len(msg))
			}
		}
	})
}
