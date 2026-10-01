package link

import (
	"testing"

	"github.com/halpworld/halpwords/pkg/words"
)

func TestMergeClampsServerBox(t *testing.T) {
	local := words.NewMemory()
	server := &words.Memory{Clock: 5, Cards: map[string]*words.Card{
		"a = x": {Box: 9, Seen: 1},
		"b = y": {Box: -3, Seen: 1},
		"c = z": nil,
	}}
	merge(local, server, nil, "fr")
	if got := local.Cards["a = x"].Box; got != words.Boxes {
		t.Fatalf("box %d, want %d", got, words.Boxes)
	}
	if got := local.Cards["b = y"].Box; got != 0 {
		t.Fatalf("box %d, want 0", got)
	}
	local.Summarize([]words.Entry{{Prompt: "a", Answers: []string{"x"}}})
}
