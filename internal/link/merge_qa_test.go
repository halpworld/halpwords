package link

import (
	"testing"

	"github.com/halpworld/halpwords/pkg/words"
)

func qaMerge(localSeen, serverSeen int, queued []qAnswer) *words.Card {
	local := &words.Memory{Clock: 20, Cards: map[string]*words.Card{"a = x": {Box: 4, Seen: localSeen, Due: 70}}}
	server := &words.Memory{Clock: 10, Cards: map[string]*words.Card{"a = x": {Box: 2, Seen: serverSeen, Due: 18}}}
	merge(local, server, queued, "fr")
	return local.Cards["a = x"]
}

func TestQAMergeSeenOrdering(t *testing.T) {
	if c := qaMerge(8, 3, nil); c.Box != 4 || c.Seen != 8 {
		t.Errorf("local > server: %+v", c)
	}
	if c := qaMerge(2, 3, nil); c.Box != 2 || c.Seen != 3 {
		t.Errorf("local < server: %+v", c)
	}
	// equal: server wins (ties go to the server)
	if c := qaMerge(3, 3, nil); c.Box != 2 || c.Seen != 3 {
		t.Errorf("equal: %+v", c)
	}
}

func TestQAMergeQueueReplay(t *testing.T) {
	q := []qAnswer{
		{Lang: "fr", Word: "a = x", Tier: int(words.Perfect)},
		{Lang: "fr", Word: "a = x", Tier: int(words.Perfect)},
		{Lang: "de", Word: "a = x", Tier: int(words.Perfect)}, // other language: ignored
	}
	// Local already counted the 2 queued answers on top of server's 3: Seen 5.
	// Replay gives server 3 + 2 = 5, equal, so the replayed card is taken.
	c := qaMerge(5, 3, q)
	if c.Seen != 5 || c.Box != 4 {
		t.Errorf("replay equal: %+v", c)
	}
	// Local played 3 more offline besides the queue: Seen 8 > 5, local stays.
	if c := qaMerge(8, 3, q); c.Seen != 8 || c.Box != 4 {
		t.Errorf("replay, local newer: %+v", c)
	}
	// Server saw other-device answers: 9 > local 5, replay on top of server.
	if c := qaMerge(5, 9, q); c.Seen != 11 {
		t.Errorf("replay, server newer: %+v", c)
	}
}

func TestQAMergeReplayClampsBadServerBox(t *testing.T) {
	local := &words.Memory{Clock: 3, Cards: map[string]*words.Card{}}
	server := &words.Memory{Clock: 1, Cards: map[string]*words.Card{"a = x": {Box: 500, Seen: 1}}}
	merge(local, server, []qAnswer{{Lang: "fr", Word: "a = x", Tier: int(words.Perfect)}}, "fr")
	if b := local.Cards["a = x"].Box; b != words.Boxes {
		t.Errorf("box %d", b)
	}
}
