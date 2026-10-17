package serverurl

import (
	"bufio"
	"os"
	"regexp"
	"strings"
	"testing"
)

// serverRE is SERVER_RE from the Makefile: the addresses
// `make build-web SERVER=…` writes into the web page.
func serverRE(t *testing.T) *regexp.Regexp {
	t.Helper()
	f, err := os.Open("../../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "SERVER_RE"); ok {
			v = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(v), ":="))
			return regexp.MustCompile(strings.ReplaceAll(v, "$$", "$"))
		}
	}
	t.Fatal("no SERVER_RE in the Makefile")
	return nil
}

// TestBuildWebServer checks a sample: each address below that the
// Makefile lets into a web build is one the game accepts (so SERVER
// shouldn't name a server the game would quietly ignore), and each
// address in the second list, which could break out of the page's script,
// is refused. It can't check every address the regex takes, so keep the
// regex simple and add a case when it changes.
func TestBuildWebServer(t *testing.T) {
	re := serverRE(t)
	for _, s := range []string{
		"https://test.halpwords.com",
		"https://halpwords.com",
		"https://staging.example.org:8443",
		"https://example.org/halpwords",
		"http://localhost:8080",
		"http://127.0.0.1:8080",
		"http://[::1]:8080",
	} {
		if !re.MatchString(s) {
			t.Errorf("SERVER_RE refuses %q", s)
		}
		if got, ok := Clean(s); !ok || got != s {
			t.Errorf("Clean(%q) = %q, %v; the web build would be ignored", s, got, ok)
		}
	}
	for _, s := range []string{
		"http://test.halpwords.com",
		"http://localhost.evil",
		`https://x.example";alert(1);"`,
		"https://x.example'",
		"https://x.example</script>",
		`https://x.example\`,
		"https://x.example|y",
		"https://x.example&y",
		"https://x.example?a=b",
		"https://x.example#f",
		"https://u:p@x.example",
		"https://x.example\n",
		"javascript:alert(1)",
		"test.halpwords.com",
		"https://",
	} {
		if re.MatchString(s) {
			t.Errorf("SERVER_RE takes %q", s)
		}
	}
}

// TestWebPageServer checks the web page hands SERVER to the game: the
// Makefile fills in @SERVER@, and the page passes it to the Go runtime as
// HALPWORDS_SERVER (wasm_exec.js turns go.env into the environment that
// os.Getenv, and so FromEnv, reads) before it runs the game.
func TestWebPageServer(t *testing.T) {
	data, err := os.ReadFile("../../build/web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(data)
	set := strings.Index(page, `go.env = { `+Env+`: server }`)
	run := strings.Index(page, "go.run(")
	if !strings.Contains(page, `const server = "@SERVER@";`) || set < 0 || run < set {
		t.Errorf("build/web/index.html doesn't set go.env from @SERVER@ before go.run")
	}
	mk, err := os.ReadFile("../../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mk), "s|@SERVER@|$$SERVER_URL|g") {
		t.Errorf("the Makefile's build-web doesn't fill in @SERVER@")
	}
}
