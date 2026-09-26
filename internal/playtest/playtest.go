// Package playtest is the web game's play-test mode, for quests made in
// halpwords-server's map editor (server task W10.3). The editor's
// Play-test button opens the web game with the address of the quest's
// file in the page address:
//
//	https://play.example/?quest=https%3A%2F%2Fplay.example%2Fplaytest%2F…
//
// The game fetches that file, checks it with pkg/maps, and starts the
// quest. The file must come from the page's own origin (the same scheme,
// host and port); any other address is refused, so a link can't make the
// game fetch from a third party. While play-testing, the game keeps its
// files in memory only (save.UseMemory): nothing is saved, the player's
// own saves are left alone, and nothing is sent to an account.
//
// This package has no Ebitengine or browser code, so it is tested on
// every system; the web build reads the page address in page_js.go.
package playtest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/halpworld/halpwords/pkg/maps"
)

// Param is the page address's query parameter that holds the quest
// file's address.
const Param = "quest"

// MaxFile is the largest quest file the game fetches, in bytes. The
// server keeps quests to the same size.
const MaxFile = 512 << 10

// Timeout is how long fetching a quest may take.
const Timeout = 30 * time.Second

// Errors.
var (
	// ErrCrossOrigin: the quest's address is not on the game's own
	// origin.
	ErrCrossOrigin = errors.New("the quest must come from this game's own website")
	// ErrBadAddress: the quest's address or the page's can't be read.
	ErrBadAddress = errors.New("the quest's address is not a web address")
)

// FromPage returns the address of the quest to play-test named in page,
// the web game's own address (location.href), or nil when page names
// none. The quest's address may be relative to page. It must be on
// page's origin: http or https, the same host and port, and no user name
// or password.
func FromPage(page string) (*url.URL, error) {
	p, err := url.Parse(page)
	if err != nil || (p.Scheme != "https" && p.Scheme != "http") || p.Host == "" {
		return nil, ErrBadAddress
	}
	raw := p.Query().Get(Param)
	if raw == "" {
		return nil, nil
	}
	return SameOrigin(p, raw)
}

// SameOrigin resolves raw against the page's address and returns it if
// it is on the page's origin.
func SameOrigin(page *url.URL, raw string) (*url.URL, error) {
	// A backslash is a slash to browsers ("/\evil.example" is another
	// host), and control characters are dropped by them; refuse both
	// rather than guess.
	if strings.ContainsAny(raw, "\\") || strings.IndexFunc(raw, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return nil, ErrBadAddress
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return nil, ErrBadAddress
	}
	u := page.ResolveReference(ref)
	if u.User != nil || ref.User != nil {
		return nil, ErrCrossOrigin
	}
	if !strings.EqualFold(u.Scheme, page.Scheme) || !strings.EqualFold(u.Host, page.Host) || u.Opaque != "" {
		return nil, ErrCrossOrigin
	}
	u.Scheme, u.Host = page.Scheme, page.Host
	u.Fragment, u.RawFragment = "", ""
	return u, nil
}

// Fetch gets the quest at u with client and checks it. u should come
// from FromPage. The request carries no cookies or other credentials in
// the web build, and a redirect is refused.
func Fetch(ctx context.Context, client *http.Client, u *url.URL) (*maps.Quest, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, ErrBadAddress
	}
	req.Header.Set("Accept", "application/json")
	browserOptions(req)
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch the quest: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusOK:
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusGone:
		return nil, errors.New("the play-test link has run out or the quest is gone; press Play-test again")
	default:
		return nil, fmt.Errorf("couldn't fetch the quest (%s)", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxFile+1))
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch the quest: %w", err)
	}
	if len(data) > MaxFile {
		return nil, errors.New("the quest file is too big")
	}
	return Load(data)
}

// Load reads and checks a fetched quest file.
func Load(data []byte) (*maps.Quest, error) {
	q, err := maps.Load("playtest."+maps.QuestFormat, data)
	if err != nil {
		return nil, err
	}
	if ps := q.Check(nil); len(ps) > 0 {
		return nil, fmt.Errorf("the quest has a problem: %s", ps[0])
	}
	return q, nil
}
