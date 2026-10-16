// Package serverurl decides which Halpwords server the game talks to, for
// the link (device tokens) and for reports. One rule for both: tokens and
// reports never travel unencrypted to another computer.
package serverurl

import (
	"net/url"
	"os"
	"strings"
)

// Env is the environment variable that names another server.
const Env = "HALPWORDS_SERVER"

// Clean returns raw without a trailing slash and true if it is a safe
// server address: https, or http only to this computer (localhost,
// 127.0.0.1 or [::1], for development), with a host, and without
// credentials, a query or a fragment. Otherwise it returns "" and false.
func Clean(raw string) (string, bool) {
	s := strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(s)
	if s == "" || err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	local := host == "localhost" || host == "127.0.0.1" || host == "::1"
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return "", false
	}
	return s, true
}

// FromEnv is HALPWORDS_SERVER if it is a safe address (see Clean), else
// def. It has no trailing slash. A refused setting is quietly ignored, so
// the game works as if it were not set.
func FromEnv(def string) string {
	if s, ok := Clean(os.Getenv(Env)); ok {
		return s
	}
	return def
}
