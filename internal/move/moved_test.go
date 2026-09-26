package move

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const movedJS = "../../web/moved/moved.js"

// The moved page must send players to the same addresses as the game.
func TestMovedPageAddresses(t *testing.T) {
	src, err := os.ReadFile(movedJS)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`const OLD_URL = "` + OldURL + `"`,
		`const PLAY_URL = "` + PlayURL + `"`,
		fmt.Sprintf("const MAX_PART = %d;", MaxPart),
	} {
		if !bytes.Contains(src, []byte(want)) {
			t.Errorf("moved.js lacks %s", want)
		}
	}
	html, err := os.ReadFile("../../web/moved/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(html, []byte(`href="`+PlayURL+`"`)) {
		t.Errorf("index.html doesn't link to %s", PlayURL)
	}
}

// nodeScript runs the moved page's code in Node: it reads a fake local
// storage from the file in argv[3] (argv[2] is the page's folder), and prints what the page would send.
const nodeScript = `
const m = require(process.argv[2] + "/moved.js");
const fs = require("fs");
(async () => {
  const input = JSON.parse(fs.readFileSync(process.argv[3], "utf8"));
  const keys = Object.keys(input.storage);
  const storage = { length: keys.length, key: (i) => keys[i], getItem: (k) => input.storage[k] };
  const files = m.collect(storage);
  const out = {
    exportable: input.names.map(m.exportable),
    targets: await Promise.all([1, 2, 3, 4].map((k) => m.target(storage, k))),
    split: await m.fragments(await m.encode(files), input.size),
    empty: await m.target({ length: 0, key: () => null, getItem: () => null }, 1),
  };
  process.stdout.write(JSON.stringify(out));
})().catch((e) => { console.error(e); process.exit(1); });
`

// The page's JavaScript and the game's Go agree: what the page sends, the
// game reads back, whole or in parts.
func TestMovedPageInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	dir := t.TempDir()
	abs, err := filepath.Abs(filepath.Dir(movedJS))
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "run.js")
	os.WriteFile(script, []byte(nodeScript), 0o644)

	files := saves()
	noise := make([]byte, 768<<10)
	rand.Read(noise)
	files["words/huge.txt"] = []byte(base64.StdEncoding.EncodeToString(noise)) // over 1 MB
	storage := map[string]string{"other-game/x": "not ours"}
	for n, d := range files {
		storage["halpwords/"+n] = string(d)
	}
	var names []string
	for n := range saves() {
		names = append(names, n)
	}
	names = append(names, "", "/abs", "a//b", "a\\b", "a\nb", "a/./b")
	in, _ := json.Marshal(map[string]any{"storage": storage, "names": names, "size": 300_000})
	inFile := filepath.Join(dir, "in.json")
	os.WriteFile(inFile, in, 0o644)

	cmd := exec.Command(node, script, abs, inFile)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	outJSON, err := cmd.Output()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, stderr.String())
	}
	var out struct {
		Exportable []bool
		Targets    []string
		Split      []string
		Empty      string
	}
	if err := json.Unmarshal(outJSON, &out); err != nil {
		t.Fatalf("%v: %s", err, outJSON)
	}

	for i, n := range names {
		if out.Exportable[i] != Exportable(n) {
			t.Errorf("exportable(%q): JavaScript says %v, Go %v", n, out.Exportable[i], Exportable(n))
		}
	}
	if out.Empty != PlayURL {
		t.Errorf("with no saves the page goes to %q", out.Empty)
	}

	want := map[string][]byte{}
	for n, d := range files {
		if Exportable(n) {
			want[n] = d
		}
	}
	// The page sends part k when asked for it (#send=k), and the game
	// asks for each part in turn.
	st := memStore{}
	var got *Incoming
	visits := 0
	for k, tries := 1, 0; got == nil; tries++ {
		visits++
		if tries > len(out.Targets) || k > len(out.Targets) {
			t.Fatal("never finished")
		}
		u := out.Targets[k-1]
		if strings.Contains(u, "sk-secret") {
			t.Fatal("the AI keys are in the address")
		}
		if len(u) > 1<<20 {
			t.Fatalf("an address of %d characters", len(u))
		}
		frag, ok := strings.CutPrefix(u, PlayURL+"#")
		if !ok {
			t.Fatalf("the page goes to %.80q", u)
		}
		var err error
		if got, k, err = Receive(st, frag); err != nil {
			t.Fatal(err)
		}
	}
	if visits < 2 {
		t.Fatalf("a save over 1 MB came in %d visit", visits)
	}
	if !maps.EqualFunc(got.Files, want, bytes.Equal) {
		t.Fatal("the saves changed on the way")
	}

	// With smaller parts too.
	if len(out.Split) < 3 {
		t.Fatalf("%d parts", len(out.Split))
	}
	var parts []Part
	for _, f := range out.Split {
		p, ok, err := ParseFragment(f)
		if !ok || err != nil {
			t.Fatalf("part %.60q: %v", f, err)
		}
		parts = append(parts, p)
	}
	got = receiveAll(t, memStore{}, parts)
	if !maps.EqualFunc(got.Files, want, bytes.Equal) {
		t.Fatal("the saves changed on the way, in parts")
	}
}
