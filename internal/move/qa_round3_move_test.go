package move

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/halpworld/halpwords/internal/profile"
	"github.com/halpworld/halpwords/internal/save"
)

// TestQA3ImportWithCurrentLearnerThenLoad: importing while a learner's
// folder is the current one writes at the root, and the imported list and
// learner then load, with the learner's files in their own folder.
func TestQA3ImportWithCurrentLearnerThenLoad(t *testing.T) {
	save.UseMemory()
	save.Use("profiles/cur")
	t.Cleanup(func() { save.Use(save.Root) })
	// Something of the current learner's that the import replaces.
	if err := save.Root.Write("profiles/cur/progress.json", []byte(`{"XP":1}`)); err != nil {
		t.Fatal(err)
	}
	list := `{"List":[{"ID":"abc123","Name":"Aoife"}],"Current":"abc123","Migrated":true}`
	files := map[string][]byte{
		"learners.json":                     []byte(list),
		"profiles/abc123/progress.json":     []byte(`{"XP":42}`),
		"profiles/abc123/words/animaux.txt": []byte("title: Animaux\n"),
	}
	in := &Incoming{ID: "0123456789abcdef", Files: files}
	if err := in.Apply(Saves); err != nil {
		t.Fatal(err)
	}
	if _, err := save.Root.Read("profiles/cur/progress.json"); err == nil {
		t.Error("the current learner's file that isn't in the import is left")
	}
	ls, err := profile.OpenLearners(save.Root)
	if err != nil {
		t.Fatal(err)
	}
	if ls.Repaired {
		t.Error("the imported list was taken as damaged")
	}
	l := ls.CurrentLearner()
	if l == nil || l.ID != "abc123" || l.Name != "Aoife" {
		t.Fatalf("current learner = %+v", l)
	}
	if err := ls.Use(l.ID); err != nil {
		t.Fatal(err)
	}
	got, err := save.Read("progress.json")
	if err != nil || string(got) != `{"XP":42}` {
		t.Errorf("progress.json of the loaded learner = %q, %v", got, err)
	}
	if got, err := save.Read("words/animaux.txt"); err != nil || !bytes.Contains(got, []byte("Animaux")) {
		t.Errorf("words/animaux.txt of the loaded learner = %q, %v", got, err)
	}
}

// nodeCollect runs collect and exportable over a fake storage in argv[3].
const nodeCollect = `
const m = require(process.argv[2] + "/moved.js");
const fs = require("fs");
const input = JSON.parse(fs.readFileSync(process.argv[3], "utf8"));
const keys = Object.keys(input.storage);
const storage = { length: keys.length, key: (i) => keys[i], getItem: (k) => input.storage[k] };
process.stdout.write(JSON.stringify(m.collect(storage)));
`

// TestQA3CollectLeavesSecretsBehind: collect() over a storage holding
// every file that stays behind, at the root and in a learner's folder,
// outputs none of them (and no secret), and every file the game's
// Exportable lets move.
func TestQA3CollectLeavesSecretsBehind(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	stay := []string{"ai.json", "link.json", "link-queue.json", "link-parked.json", "crash.txt"}
	storage := map[string]string{
		"other-game/ai.json": "sk-secret-other",
		"halpwords":          "no slash after the prefix",
	}
	var wantNames []string
	add := func(name string) {
		storage["halpwords/"+name] = "data of " + name
		if Exportable(name) {
			wantNames = append(wantNames, name)
		}
	}
	for _, d := range []string{"", "profiles/k3v9q2/", "profiles/zzzzzz/words/", "words/"} {
		for _, f := range stay {
			n := d + f
			storage["halpwords/"+n] = "sk-secret " + n
			if Exportable(n) {
				t.Fatalf("Go lets %s move", n)
			}
			storage["halpwords/"+n+".bad"] = "sk-secret " + n + ".bad"
		}
	}
	storage["halpwords/move/part-x-1-1"] = "sk-secret stash"
	storage["halpwords/profiles/k3v9q2/memory.json.bad"] = "sk-secret bad"
	for _, n := range []string{"adventure.json", "learners.json", "profiles/k3v9q2/adventure.json", "profiles/k3v9q2/words/x.txt", "ai/bank-fr.json", "ai-spend.json", "words/a.txt"} {
		add(n)
	}
	dir := t.TempDir()
	abs, err := filepath.Abs(filepath.Dir(movedJS))
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "run.js")
	inFile := filepath.Join(dir, "in.json")
	in, _ := json.Marshal(map[string]any{"storage": storage})
	if err := os.WriteFile(script, []byte(nodeCollect), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inFile, in, 0o644); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd := exec.Command(node, script, abs, inFile)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, stderr.String())
	}
	if strings.Contains(string(out), "secret") {
		t.Errorf("a secret is in the output: %s", out)
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
	slices.Sort(wantNames)
	if !slices.Equal(gotNames, wantNames) {
		t.Errorf("collect gives %v, want %v", gotNames, wantNames)
	}
}
