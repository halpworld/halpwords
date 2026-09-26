// Package move carries a web player's progress from one address to another.
//
// A browser keeps local storage per address, so saves made at the old
// address (OldURL) can't be seen at the new one (PlayURL). The old address
// serves a small "We've moved" page (web/moved) that reads the saves and
// opens the new address with them in the URL fragment, which browsers never
// send to a server:
//
//	https://play.halpwords.com/#import=1/1:<id>:<data>
//
// <data> is the payload: "z" and the base64url (no padding) of the raw
// DEFLATE of a JSON bundle {"v":1,"files":{"name":"contents",...}}, or "j"
// and the base64url of the JSON itself when the browser can't compress.
// <id> is the first 16 hex digits of the SHA-256 of <data>. A payload
// longer than MaxPart is split into parts, 1/n to n/n; the game keeps each
// part and sends the player back to the old page for the next one
// (OldURL#send=k) until it has them all.
//
// The game imports a payload once: it asks before replacing progress that
// is already in the browser, remembers the ids it has seen, and clears the
// fragment. The logic here is plain Go; browser.go and browser_js.go hold
// the few calls into the page.
package move

import (
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// The web game's addresses. WebURL is the one players are sent to today:
// README.md, docs/RELEASING.md and the game use it, and a test checks they
// agree. At the cut-over (docs/RELEASING.md) it becomes PlayURL.
const (
	// OldURL is where the web game was first served, on GitHub Pages.
	OldURL = "https://halpworld.github.io/halpwords/"
	// PlayURL is the web game's own address.
	PlayURL = "https://play.halpwords.com/"
	// WebURL is where to play the web game now.
	WebURL = OldURL
)

const (
	// Key starts the fragment that carries a payload: #import=k/n:id:data.
	Key = "import="
	// SendKey starts the fragment that asks the old page for part k:
	// #send=k.
	SendKey = "send="
	// MaxPart is the most payload characters in one fragment. Firefox
	// refuses URLs over 1 MiB; Chrome allows 2 MB.
	MaxPart = 900_000
	// MaxParts is the most parts a payload may have.
	MaxParts = 64
	// maxBundle caps the JSON a payload may inflate to. A browser's local
	// storage holds about 5 MB.
	maxBundle = 32 << 20
	version   = 1
)

// Exportable reports whether the file name moves with the player. Not
// moved: ai.json, which holds the player's AI keys, and link.json and
// link-queue.json, which hold the link's tokens and the events waiting to be
// sent (a URL can end up in the browser's history; the game links again at
// its new address); crash reports; damaged files kept to one side; and
// this package's own files.
func Exportable(name string) bool {
	switch {
	case !validName(name),
		name == "ai.json",
		name == "link.json",
		name == "link-queue.json",
		name == "crash.txt",
		strings.HasSuffix(name, ".bad"),
		strings.HasPrefix(name, stashDir):
		return false
	}
	return true
}

// validName reports whether name is a clean relative file name: forward
// slashes, no "." or ".." parts, no control characters.
func validName(name string) bool {
	if name == "" || len(name) > 255 || strings.HasPrefix(name, "/") || path.Clean(name) != name {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "." || part == ".." {
			return false
		}
	}
	for _, r := range name {
		if r == '\\' || !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

type bundle struct {
	V     int               `json:"v"`
	Files map[string]string `json:"files"`
}

// Encode packs the files that move into a payload.
func Encode(files map[string][]byte) (string, error) {
	b := bundle{V: version, Files: map[string]string{}}
	for name, data := range files {
		if Exportable(name) {
			b.Files[name] = string(data)
		}
	}
	js, err := json.Marshal(b) // map keys come out sorted
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.BestCompression)
	if err != nil {
		return "", err
	}
	w.Write(js)
	if err := w.Close(); err != nil {
		return "", err
	}
	return "z" + base64.RawURLEncoding.EncodeToString(buf.Bytes()), nil
}

// Decode unpacks a payload. Files that don't move are left out.
func Decode(payload string) (map[string][]byte, error) {
	if payload == "" {
		return nil, errors.New("empty payload")
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload[1:])
	if err != nil {
		return nil, fmt.Errorf("payload: %w", err)
	}
	var js []byte
	switch payload[0] {
	case 'z':
		js, err = io.ReadAll(io.LimitReader(flate.NewReader(bytes.NewReader(raw)), maxBundle+1))
		if err != nil {
			return nil, fmt.Errorf("payload: %w", err)
		}
	case 'j':
		js = raw
	default:
		return nil, fmt.Errorf("payload: unknown format %q", payload[0])
	}
	if len(js) > maxBundle {
		return nil, errors.New("payload: too big")
	}
	var b bundle
	if err := json.Unmarshal(js, &b); err != nil {
		return nil, fmt.Errorf("payload: %w", err)
	}
	if b.V != version {
		return nil, fmt.Errorf("payload: from a newer game (version %d)", b.V)
	}
	files := map[string][]byte{}
	for name, s := range b.Files {
		if Exportable(name) {
			files[name] = []byte(s)
		}
	}
	return files, nil
}

// ID names a payload, so its parts can be told from another's.
func ID(payload string) string {
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:8])
}

// Part is one fragment's share of a payload.
type Part struct {
	K, N int    // part K of N, from 1
	ID   string // the payload's ID
	Data string
}

// Fragment is the part as a URL fragment, without the "#".
func (p Part) Fragment() string {
	return fmt.Sprintf("%s%d/%d:%s:%s", Key, p.K, p.N, p.ID, p.Data)
}

// Split cuts a payload into parts of at most size characters each.
func Split(payload string, size int) []Part {
	n := (len(payload) + size - 1) / size
	n = max(n, 1)
	id := ID(payload)
	parts := make([]Part, n)
	for k := range parts {
		end := min(len(payload), (k+1)*size)
		parts[k] = Part{K: k + 1, N: n, ID: id, Data: payload[k*size : end]}
	}
	return parts
}

// ParseFragment reads a part from a URL fragment, with or without its "#".
// ok is false when the fragment isn't an import at all.
func ParseFragment(frag string) (p Part, ok bool, err error) {
	frag = strings.TrimPrefix(frag, "#")
	rest, ok := strings.CutPrefix(frag, Key)
	if !ok {
		return Part{}, false, nil
	}
	bad := func() (Part, bool, error) { return Part{}, true, errors.New("the progress link is damaged") }
	head, data, ok1 := strings.Cut(rest, ":")
	id, data, ok2 := strings.Cut(data, ":")
	ks, ns, ok3 := strings.Cut(head, "/")
	if !ok1 || !ok2 || !ok3 || len(id) != 16 || strings.Trim(id, "0123456789abcdef") != "" {
		return bad()
	}
	k, err1 := strconv.Atoi(ks)
	n, err2 := strconv.Atoi(ns)
	if err1 != nil || err2 != nil || n < 1 || n > MaxParts || k < 1 || k > n || data == "" {
		return bad()
	}
	return Part{K: k, N: n, ID: id, Data: data}, true, nil
}

// Join puts parts back together into their payload, checking they are all
// there and that the result matches their ID.
func Join(parts []Part) (string, error) {
	if len(parts) == 0 {
		return "", errors.New("no parts")
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].K < parts[j].K })
	n, id := parts[0].N, parts[0].ID
	if len(parts) != n {
		return "", fmt.Errorf("have %d of %d parts", len(parts), n)
	}
	var sb strings.Builder
	for i, p := range parts {
		if p.K != i+1 || p.N != n || p.ID != id {
			return "", errors.New("the parts don't belong together")
		}
		sb.WriteString(p.Data)
	}
	payload := sb.String()
	if ID(payload) != id {
		return "", errors.New("the progress link is damaged")
	}
	return payload, nil
}
