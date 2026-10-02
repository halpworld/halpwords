//go:build !js

package profile

import (
	"testing"

	"github.com/halpworld/halpwords/internal/save"
	"github.com/halpworld/halpwords/pkg/words"
)

func TestQALoadClampsBoxesAndDropsNilCards(t *testing.T) {
	useTempDir(t)
	f := save.Root
	f.Write(memoryFile, []byte(`{"Memory":{"fr":{"Clock":4,"Cards":{"a = x":{"Box":99,"Seen":2},"b = y":{"Box":-7,"Seen":1},"c = z":null}},"la":null,"de":{}}}`))
	p, errs := LoadFrom(f)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	m := p.Memory["fr"]
	if m == nil || m.Cards["a = x"].Box != words.Boxes || m.Cards["b = y"].Box != 0 {
		t.Fatalf("%+v", m)
	}
	if _, ok := m.Cards["c = z"]; ok {
		t.Error("nil card kept")
	}
	if p.Memory["de"] == nil || p.Memory["de"].Cards == nil {
		t.Error("empty memory not initialised")
	}
	m.Summarize([]words.Entry{{Prompt: "a", Answers: []string{"x"}}, {Prompt: "b", Answers: []string{"y"}}})
}
