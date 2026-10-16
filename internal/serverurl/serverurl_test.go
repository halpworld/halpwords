package serverurl

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClean(t *testing.T) {
	for in, want := range map[string]string{
		"https://halpwords.com":          "https://halpwords.com",
		"https://staging.halpwords.com/": "https://staging.halpwords.com",
		"  https://example.org:8443  ":   "https://example.org:8443",
		"http://localhost":               "http://localhost",
		"http://localhost:8080":          "http://localhost:8080",
		"http://127.0.0.1:8080":          "http://127.0.0.1:8080",
		"http://[::1]:8080":              "http://[::1]:8080",
		"HTTP://LOCALHOST:8080":          "HTTP://LOCALHOST:8080",
		// Refused.
		"":                        "",
		"http://halpwords.com":    "",
		"http://evil.example":     "",
		"http://localhost.evil":   "",
		"http://127.0.0.2:8080":   "",
		"ftp://localhost":         "",
		"//halpwords.com":         "",
		"halpwords.com":           "",
		"https://":                "",
		"https://u:p@x.example":   "",
		"https://x.example?a=b":   "",
		"https://x.example/#frag": "",
		"not a url":               "",
		"http://[::1":             "",
		"javascript:alert(1)":     "",
	} {
		got, ok := Clean(in)
		if got != want || ok != (want != "") {
			t.Errorf("Clean(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
}

func TestFromEnv(t *testing.T) {
	const def = "https://default.example"
	for env, want := range map[string]string{
		"":                         def,
		"https://staging.example/": "https://staging.example",
		"http://localhost:9":       "http://localhost:9",
		"http://remote.example":    def,
		"garbage":                  def,
	} {
		t.Setenv(Env, env)
		if got := FromEnv(def); got != want {
			t.Errorf("%s=%q: %q, want %q", Env, env, got, want)
		}
	}
}

// A local httptest server is allowed, and is the only plain-http target
// the game will talk to.
func TestHTTPTestServerIsLocal(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	if got, ok := Clean(srv.URL); !ok || got != srv.URL {
		t.Errorf("Clean(%q) = %q, %v", srv.URL, got, ok)
	}
	tls := httptest.NewTLSServer(http.NotFoundHandler())
	defer tls.Close()
	if _, ok := Clean(tls.URL); !ok {
		t.Errorf("Clean(%q) refused an https server", tls.URL)
	}
}
