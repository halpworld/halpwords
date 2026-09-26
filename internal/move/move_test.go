package move

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"io/fs"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// memStore is a Store in memory.
type memStore map[string][]byte

func (m memStore) Read(name string) ([]byte, error) {
	d, ok := m[name]
	if !ok {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrNotExist}
	}
	return d, nil
}
func (m memStore) Write(name string, data []byte) error { m[name] = bytes.Clone(data); return nil }
func (m memStore) Remove(name string) error             { delete(m, name); return nil }
func (m memStore) All() ([]string, error)               { return slices.Sorted(maps.Keys(m)), nil }

// saves looks like a web player's local storage.
func saves() map[string][]byte {
	return map[string][]byte{
		"adventure.json":     []byte(`{"Version":2,"Language":"fr","Class":1}`),
		"settings.json":      []byte(`{"Game":{"Music":5}}`),
		"memory.json":        []byte(`{"Memory":{"fr":{"Cards":{"le chat":{"Box":3}}}}}`),
		"fame.json":          []byte(`{"Name":"Aoife","Runs":[]}`),
		"words/animaux.txt":  []byte("title: Animaux\nlanguage: fr\n\ndog = le chien\n"),
		"ai-spend.json":      []byte(`{}`),
		"ai/bank-fr.json":    []byte(`{"riddles":["ünïcödé ἀγαθός"]}`),
		"ai.json":            []byte(`{"Keys":{"anthropic":"sk-secret"}}`),
		"crash.txt":          []byte("panic"),
		"memory.json.bad":    []byte("{"),
		"move/done":          []byte("0123456789abcdef\n"),
		"move/part-x-1-1":    []byte("z"),
		"../escape.json":     []byte("no"),
		"words/../../x.json": []byte("no"),
	}
}

// moved is what should arrive: everything but the files that stay.
func moved() map[string][]byte {
	m := saves()
	for n := range m {
		if !Exportable(n) {
			delete(m, n)
		}
	}
	return m
}

func TestExportable(t *testing.T) {
	for name, want := range map[string]bool{
		"adventure.json": true, "words/x.txt": true, "ai/bank-fr.json": true, "ai-spend.json": true,
		"ai.json": false, "link.json": false, "link-queue.json": false, "crash.txt": false, "fame.json.bad": false, "move/done": false,
		"": false, "/abs": false, "../x": false, "a/../b": false, "a//b": false, "a\\b": false, "a\nb": false,
	} {
		if got := Exportable(name); got != want {
			t.Errorf("Exportable(%q) = %v, want %v", name, got, want)
		}
	}
	if _, ok := moved()["ai.json"]; ok {
		t.Fatal("the AI keys would move")
	}
}

func TestRoundTrip(t *testing.T) {
	payload, err := Encode(saves())
	if err != nil {
		t.Fatal(err)
	}
	if payload[0] != 'z' || strings.Contains(payload, "sk-secret") {
		t.Fatalf("payload %q", payload)
	}
	got, err := Decode(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !maps.EqualFunc(got, moved(), bytes.Equal) {
		t.Fatalf("got %q\nwant %q", got, moved())
	}
}

func TestDecodePlainJSON(t *testing.T) {
	payload := "j" + base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"files":{"settings.json":"{}","ai.json":"k"}}`))
	got, err := Decode(payload)
	if err != nil || len(got) != 1 || string(got["settings.json"]) != "{}" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestDecodeRefusesBadPayloads(t *testing.T) {
	for _, p := range []string{"", "x", "zzzz", "z!!", "j" + base64.RawURLEncoding.EncodeToString([]byte(`{"v":2,"files":{}}`))} {
		if _, err := Decode(p); err == nil {
			t.Errorf("Decode(%q) worked", p)
		}
	}
}

func TestFragments(t *testing.T) {
	payload := strings.Repeat("abcdefghij", 25)
	parts := Split(payload, 100)
	if len(parts) != 3 {
		t.Fatalf("%d parts, want 3", len(parts))
	}
	var back []Part
	for _, p := range slices.Backward(parts) {
		got, ok, err := ParseFragment("#" + p.Fragment())
		if !ok || err != nil || got != p {
			t.Fatalf("parse %q: %+v %v %v", p.Fragment(), got, ok, err)
		}
		back = append(back, got)
	}
	joined, err := Join(back)
	if err != nil || joined != payload {
		t.Fatalf("join: %q, %v", joined, err)
	}
	if _, err := Join(back[:2]); err == nil {
		t.Fatal("joined with a part missing")
	}
	bad := slices.Clone(parts)
	bad[1].Data = "tampered"
	if _, err := Join(bad); err == nil {
		t.Fatal("joined tampered parts")
	}

	for _, f := range []string{"", "#", "play", "send=2"} {
		if _, ok, err := ParseFragment(f); ok || err != nil {
			t.Errorf("ParseFragment(%q) = %v, %v; want not an import", f, ok, err)
		}
	}
	id := ID("x")
	for _, f := range []string{"import=", "import=1/1:" + id, "import=0/1:" + id + ":x", "import=2/1:" + id + ":x",
		"import=1/99:" + id + ":x", "import=1/1:nothex:x", "import=a/b:" + id + ":x", "import=1/1:" + id + ":"} {
		if _, ok, err := ParseFragment(f); !ok || err == nil {
			t.Errorf("ParseFragment(%q) = %v, %v; want damaged", f, ok, err)
		}
	}
}

// receiveAll plays the moved page and the game: the game takes each
// fragment and asks for the next part until it has them all.
func receiveAll(t *testing.T, st Store, parts []Part) *Incoming {
	t.Helper()
	k := 1
	for range len(parts) + 1 {
		in, next, err := Receive(st, "#"+parts[k-1].Fragment())
		if err != nil {
			t.Fatal(err)
		}
		if in != nil {
			return in
		}
		if next == 0 {
			t.Fatal("nothing arrived")
		}
		if SendURL(next) != OldURL+"#send="+strconv.Itoa(next) {
			t.Fatalf("send URL %q", SendURL(next))
		}
		k = next
	}
	t.Fatal("never finished")
	return nil
}

func TestReceiveIntoEmptyBrowser(t *testing.T) {
	payload, err := Encode(saves())
	if err != nil {
		t.Fatal(err)
	}
	st := memStore{}
	in := receiveAll(t, st, Split(payload, MaxPart))
	if here, _ := Conflicts(st); len(here) != 0 {
		t.Fatalf("conflicts in an empty browser: %v", here)
	}
	if err := in.Apply(st); err != nil {
		t.Fatal(err)
	}
	for n, want := range moved() {
		if !bytes.Equal(st[n], want) {
			t.Errorf("%s = %q, want %q", n, st[n], want)
		}
	}
	for n := range st {
		if strings.HasPrefix(n, "move/part-") {
			t.Errorf("left %s behind", n)
		}
	}
	// Following the same link again doesn't import twice.
	in, next, err := Receive(st, Split(payload, MaxPart)[0].Fragment())
	if in != nil || next != 0 || err != nil {
		t.Fatalf("second time: %v %d %v", in, next, err)
	}
}

func TestReceiveReplacesOrKeeps(t *testing.T) {
	payload, _ := Encode(saves())
	here := func() memStore {
		return memStore{
			"adventure.json":   []byte("new hero"),
			"words/mine.txt":   []byte("mine"),
			"ai.json":          []byte("new keys"),
			"move/part-zz-1-1": []byte("junk"),
		}
	}

	st := here()
	in := receiveAll(t, st, Split(payload, MaxPart))
	conflicts, _ := Conflicts(st)
	if !slices.Equal(conflicts, []string{"adventure.json", "words/mine.txt"}) {
		t.Fatalf("conflicts %v", conflicts)
	}
	if err := in.Apply(st); err != nil {
		t.Fatal(err)
	}
	if string(st["adventure.json"]) != string(saves()["adventure.json"]) {
		t.Fatal("the hero didn't move")
	}
	if _, ok := st["words/mine.txt"]; ok {
		t.Fatal("kept a list from the progress that was replaced")
	}
	if string(st["ai.json"]) != "new keys" {
		t.Fatal("the AI keys here were touched")
	}

	st = here()
	in = receiveAll(t, st, Split(payload, MaxPart))
	if err := in.Discard(st); err != nil {
		t.Fatal(err)
	}
	if string(st["adventure.json"]) != "new hero" || string(st["words/mine.txt"]) != "mine" {
		t.Fatal("keeping changed the progress here")
	}
	if in, next, _ := Receive(st, Split(payload, MaxPart)[0].Fragment()); in != nil || next != 0 {
		t.Fatal("asked again after keeping")
	}
}

// A save bigger than 1 MB, which doesn't compress, still moves in parts.
func TestBigSaveMovesInParts(t *testing.T) {
	noise := make([]byte, 1536<<10)
	rand.Read(noise)
	files := saves()
	files["words/huge.txt"] = []byte(base64.StdEncoding.EncodeToString(noise)) // 2 MB of text
	payload, err := Encode(files)
	if err != nil {
		t.Fatal(err)
	}
	parts := Split(payload, MaxPart)
	if len(parts) < 2 {
		t.Fatalf("%d parts for %d characters", len(parts), len(payload))
	}
	for _, p := range parts {
		if n := len(PlayURL) + 1 + len(p.Fragment()); n > 1<<20 {
			t.Fatalf("a URL of %d characters", n)
		}
	}
	st := memStore{}
	in := receiveAll(t, st, parts)
	if !bytes.Equal(in.Files["words/huge.txt"], files["words/huge.txt"]) {
		t.Fatal("the big file changed on the way")
	}
	if err := in.Apply(st); err != nil {
		t.Fatal(err)
	}
	if len(st["words/huge.txt"]) != len(files["words/huge.txt"]) {
		t.Fatal("the big file didn't arrive")
	}
}

// A new move replaces an unfinished one.
func TestNewerMoveWins(t *testing.T) {
	a, _ := Encode(map[string][]byte{"settings.json": []byte(strings.Repeat("a", 5000))})
	b, _ := Encode(map[string][]byte{"settings.json": []byte("b")})
	st := memStore{}
	if _, next, err := Receive(st, Split(a, 20)[0].Fragment()); next != 2 || err != nil {
		t.Fatalf("next %d, %v", next, err)
	}
	// Starting without a fragment doesn't go back for the missing part.
	if in, next, err := Receive(st, ""); in != nil || next != 0 || err != nil {
		t.Fatalf("without a fragment: %v %d %v", in, next, err)
	}
	in, _, err := Receive(st, Split(b, MaxPart)[0].Fragment())
	if err != nil || in == nil || string(in.Files["settings.json"]) != "b" {
		t.Fatalf("got %v, %v", in, err)
	}
	for n := range st {
		if strings.Contains(n, ID(a)) {
			t.Fatalf("kept %s", n)
		}
	}
}

// The README and the release notes point players at WebURL.
func TestDocsUseWebURL(t *testing.T) {
	other := PlayURL
	if WebURL == PlayURL {
		other = OldURL
	}
	for _, f := range []string{"../../README.md", "../../docs/RELEASING.md"} {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), WebURL) {
			t.Errorf("%s doesn't link to %s", f, WebURL)
		}
		if f == "../../README.md" && strings.Contains(string(data), other) {
			t.Errorf("%s still links to %s", f, other)
		}
	}
}
