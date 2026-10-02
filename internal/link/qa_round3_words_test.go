//go:build !js

package link

import (
	"strings"
	"testing"

	"github.com/halpworld/halpwords/pkg/raid"
)

// A 60-rune answer reaches the server whole; 61 is cut to 60.
func TestQA3RaidSends60RuneAnswerWhole(t *testing.T) {
	f, c, conns := playing(t)
	s := joinRoom(t, f, c, conns)
	p := c.Play()
	s.send(raidMsg)
	s.send(`{"t":"word","word":{"n":1,"prompt":"a"}}`)
	waitFor(t, p, "the word", func(s PlayState) bool { return s.Raid != nil && s.Raid.Word != nil })
	whole := strings.Repeat("é", raid.MaxAnswer)
	if !p.Answer(1, whole, false) {
		t.Fatal("the answer didn't go")
	}
	if m := s.expect("answer"); m["answer"] != whole {
		t.Fatalf("sent %d runes", len([]rune(m["answer"].(string))))
	}
	s.send(`{"t":"word","word":{"n":2,"prompt":"b"}}`)
	waitFor(t, p, "word 2", func(s PlayState) bool { return s.Raid.Word != nil && s.Raid.Word.N == 2 })
	if !p.Answer(2, whole+"x", false) {
		t.Fatal("the answer didn't go")
	}
	if m := s.expect("answer"); m["answer"] != whole {
		t.Fatalf("sent %d runes", len([]rune(m["answer"].(string))))
	}
}
