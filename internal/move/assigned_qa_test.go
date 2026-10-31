package move

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var assignedNames = []string{
	"assigned/lst_a.txt", "assigned/lst_a.audio", "assigned/sub/deeper.txt", "assigned/x",
	"profiles/k3v9q2/assigned/lst_a.txt", "profiles/k3v9q2/assigned/lst_a.audio",
	"profiles/assigned/assigned/lst.txt", "profiles/zz/assigned/a/b/c.audio",
}

// Every assigned list and audio pack stays, at the root and in any
// learner's folder; things that merely look like it still move.
func TestExportableAssignedAndLookalikes(t *testing.T) {
	for _, n := range assignedNames {
		if Exportable(n) {
			t.Errorf("Exportable(%q): an assigned file moves", n)
		}
	}
	for _, n := range []string{
		"assigned", "assigned.txt", "assigned.json", "Assigned/x.txt", "ASSIGNED/x.txt", "assigneds/x.txt",
		"words/assigned/x.txt", "ai/assigned/x.json", "profiles/assigned", "profiles/assigned/progress.json",
		"profiles/k3v9q2/words/assigned/x.txt", "profiles/k3v9q2/assigned", "my assigned/x.txt",
		"profiles/k3v9q2/memory.json", "words/lst_a.txt",
	} {
		if !Exportable(n) {
			t.Errorf("Exportable(%q): a file that should move stays", n)
		}
	}
}

// A payload from a game from before this change holds assigned lists:
// the receiver drops them, so none arrive, however it was packed.
func TestDecodeDropsAssignedFromOlderGames(t *testing.T) {
	files := map[string]string{"settings.json": "{}", "ai.json": "sk-secret"}
	for _, n := range assignedNames {
		files[n] = "licensed " + n
	}
	js, _ := json.Marshal(bundle{V: version, Files: files})
	got, err := Decode("j" + base64.RawURLEncoding.EncodeToString(js))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || string(got["settings.json"]) != "{}" {
		t.Errorf("decoded %v", got)
	}
}

// Applying leaves the receiving game's own assigned lists where they are
// (they are not "files that move"), while everything that moves is
// replaced, and a learner who comes along keeps their assigned folder.
func TestApplyLeavesAssignedFilesAlone(t *testing.T) {
	st := memStore{
		"learners.json":                        []byte(`{"List":[{"ID":"k3v9q2"}]}`),
		"profiles/k3v9q2/progress.json":        []byte(`{"XP":1}`),
		"profiles/k3v9q2/assigned/lst_a.txt":   []byte("here"),
		"profiles/k3v9q2/assigned/lst_a.audio": []byte("pack"),
		"assigned/lst_b.txt":                   []byte("root list"),
		"words/old.txt":                        []byte("goes"),
	}
	in := &Incoming{ID: "0123456789abcdef", Files: map[string][]byte{
		"learners.json":                 []byte(`{"List":[{"ID":"k3v9q2"}]}`),
		"profiles/k3v9q2/progress.json": []byte(`{"XP":99}`),
	}}
	conf, _ := Conflicts(st)
	for _, n := range conf {
		if strings.Contains(n, "assigned/") {
			t.Errorf("an assigned file is listed as a conflict: %s", n)
		}
	}
	if err := in.Apply(st); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"profiles/k3v9q2/assigned/lst_a.txt", "profiles/k3v9q2/assigned/lst_a.audio", "assigned/lst_b.txt"} {
		if _, ok := st[n]; !ok {
			t.Errorf("%s was removed by Apply", n)
		}
	}
	if _, ok := st["words/old.txt"]; ok {
		t.Error("an old own list stayed")
	}
	if string(st["profiles/k3v9q2/progress.json"]) != `{"XP":99}` {
		t.Errorf("progress %q", st["profiles/k3v9q2/progress.json"])
	}
}

// The moved page's own filter agrees with Go on assigned files and their
// lookalikes (a differential run in Node). Needs node with webcrypto for
// the other page tests, but not for this.
func TestMovedJSAgreesOnAssigned(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	names := append(slices.Clone(assignedNames),
		"assigned", "Assigned/x.txt", "words/assigned/x.txt", "profiles/assigned", "profiles/assigned/p.json",
		"profiles/k3v9q2/assigned", "assigned.txt", "profiles/k3v9q2/words/assigned/x.txt", "settings.json",
		"profiles/k3v9q2/reports/a.json", "assigned/ai.json", "profiles/x/assigned/link.json")
	storage := map[string]string{}
	var want []string
	for _, n := range names {
		storage["halpwords/"+n] = "d"
		if Exportable(n) {
			want = append(want, n)
		}
	}
	dir := t.TempDir()
	abs, _ := filepath.Abs(filepath.Dir(movedJS))
	script := filepath.Join(dir, "run.js")
	in := filepath.Join(dir, "in.json")
	data, _ := json.Marshal(map[string]any{"storage": storage})
	os.WriteFile(script, []byte(nodeCollect), 0o644)
	os.WriteFile(in, data, 0o644)
	var stderr bytes.Buffer
	cmd := exec.Command(node, script, abs, in)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("node could not run the page's code here: %v\n%s", err, stderr.String())
	}
	var got map[string]string
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	var gotNames []string
	for n := range got {
		gotNames = append(gotNames, n)
	}
	slices.Sort(gotNames)
	slices.Sort(want)
	if !slices.Equal(gotNames, want) {
		t.Errorf("JS moves %v\nGo moves %v", gotNames, want)
	}
}
