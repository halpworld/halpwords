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

func TestMergeKeepsNewerLocalCard(t *testing.T) {
	local := &words.Memory{Clock: 20, Cards: map[string]*words.Card{
		"a = x": {Box: 4, Seen: 8, Right: 8, Due: 70}, // learned offline
		"b = y": {Box: 1, Seen: 1, Due: 21},
	}}
	server := &words.Memory{Clock: 10, Cards: map[string]*words.Card{
		"a = x": {Box: 2, Seen: 3, Right: 3, Due: 18},
		"b = y": {Box: 3, Seen: 5, Right: 5, Due: 30}, // played on another device
		"c = z": {Box: 2, Seen: 2, Due: 14},
	}}
	merge(local, server, nil, "fr")
	if c := local.Cards["a = x"]; c.Box != 4 || c.Seen != 8 {
		t.Fatalf("newer local card replaced: %+v", c)
	}
	if c := local.Cards["b = y"]; c.Box != 3 || c.Seen != 5 {
		t.Fatalf("newer server card not taken: %+v", c)
	}
	if c := local.Cards["c = z"]; c == nil || c.Box != 2 {
		t.Fatalf("server-only card missing: %+v", c)
	}
}
