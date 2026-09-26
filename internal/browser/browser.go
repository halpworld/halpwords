// Package browser opens web pages from the game: the website's page for
// signing in with a school account (W5.4). On a computer it asks the
// system to open the page in the player's browser; in a web browser the
// game leaves for the page and comes back to Here.
package browser

import (
	"errors"
	"strings"
)

// ErrBadURL is an address that isn't http or https.
var ErrBadURL = errors.New("browser: not a web address")

// Open opens url, which must be an http or https address.
func Open(url string) error {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return ErrBadURL
	}
	return open(url)
}
