//go:build !js

package link

import (
	"strings"
	"testing"
	"time"

	"github.com/halpworld/halpwords/internal/typing"
	"github.com/halpworld/halpwords/pkg/raid"
	"github.com/halpworld/halpwords/pkg/words"
)

// The game's typing field holds no longer answer than a raid takes.
func TestMaxAnswerIsTheTypingField(t *testing.T) {
	if raid.MaxAnswer != typing.MaxLen {
		t.Fatalf("raid.MaxAnswer %d, typing.MaxLen %d", raid.MaxAnswer, typing.MaxLen)
	}
}

const raidMsg = `{"t":"raid","seq":5,"raid":{"boss":"Goblin King","seed":7,"lang":"fr","hp":1000,"max_hp":1000,"ends_in":600000,"raiders":2}}`

func TestPlayRaid(t *testing.T) {
	f, c, conns := playing(t)
	s := joinRoom(t, f, c, conns)
	p := c.Play()
	if p.Answer(1, "le chien", false) {
		t.Fatal("answered with no raid")
	}
	s.send(raidMsg)
	s.send(`{"t":"word","word":{"n":1,"prompt":"the dog"}}`)
	st := waitFor(t, p, "the first word", func(s PlayState) bool { return s.Raid != nil && s.Raid.Word != nil })
	if r := st.Raid; r.Number != 1 || r.Boss != "Goblin King" || r.Seed != 7 || r.Lang != "fr" || r.HP != 1000 ||
		r.Raiders != 2 || r.Word.Prompt != "the dog" || r.Word.Dodge || time.Until(r.EndsAt) < 9*time.Minute {
		t.Fatalf("raid %+v", r)
	}

	// An answer goes once, to the word the raider has.
	if p.Answer(2, "le chien", false) {
		t.Fatal("answered another word")
	}
	if !p.Answer(1, "le chien", true) {
		t.Fatal("the answer didn't go")
	}
	if m := s.expect("answer"); m["n"] != float64(1) || m["answer"] != "le chien" || m["backspace"] != true {
		t.Fatalf("sent %v", m)
	}
	if p.Answer(1, "le chien", false) {
		t.Fatal("answered the same word twice")
	}

	// The grade, the hit on the boss and the next word.
	s.send(`{"t":"graded","graded":{"n":1,"tier":4,"expected":"le chien","damage":10}}`)
	s.send(`{"t":"hit","seq":6,"id":"m2","damage":10,"boss":{"hp":990,"max_hp":1000}}`)
	s.send(`{"t":"word","word":{"n":2,"prompt":"the cat"}}`)
	st = waitFor(t, p, "the second word", func(s PlayState) bool { return s.Raid.Word != nil && s.Raid.Word.N == 2 })
	if r := st.Raid; r.HP != 990 || r.Hits != 1 || r.LastHit != (RaidHit{ID: "m2", Damage: 10}) ||
		r.Graded.Tier != words.Perfect || r.Graded.Damage != 10 {
		t.Fatalf("raid %+v, graded %+v", r, r.Graded)
	}

	// A long answer is cut to what a raid takes.
	if !p.Answer(2, strings.Repeat("é", 50), false) {
		t.Fatal("the long answer didn't go")
	}
	if m := s.expect("answer"); len([]rune(m["answer"].(string))) != raid.MaxAnswer {
		t.Fatalf("sent %d characters", len([]rune(m["answer"].(string))))
	}

	// A late raider makes the boss stronger; then it attacks.
	s.send(`{"t":"boss","seq":7,"boss":{"hp":1490,"max_hp":1500}}`)
	s.send(`{"t":"attack","seq":8,"attack":{"n":1,"of":3,"in":6000}}`)
	s.send(`{"t":"word","word":{"n":3,"prompt":"the house","dodge":true,"in":6000}}`)
	st = waitFor(t, p, "the dodge", func(s PlayState) bool { return s.Raid.Word != nil && s.Raid.Word.Dodge })
	if r := st.Raid; r.MaxHP != 1500 || r.Raiders != 3 || r.Grew != 1 || r.Attack == nil || r.Attack.Of != 3 ||
		r.Attack.Done || time.Until(r.Word.Until) < 4*time.Second {
		t.Fatalf("raid %+v, attack %+v", r, r.Attack)
	}
	s.send(`{"t":"graded","graded":{"n":3,"tier":0,"expected":"la maison","dodge":true,"late":true,"stun":5000}}`)
	s.send(`{"t":"attacked","seq":9,"attack":{"n":1,"of":3,"dodged":2}}`)
	st = waitFor(t, p, "the attack's end", func(s PlayState) bool { return s.Raid.Attack != nil && s.Raid.Attack.Done })
	if g := st.Raid.Graded; !g.Late || g.Dodged || time.Until(g.StunnedUntil) < 3*time.Second || st.Raid.Attack.Dodged != 2 ||
		st.Raid.Word != nil {
		t.Fatalf("graded %+v, attack %+v", g, st.Raid.Attack)
	}

	// The finale, and what the player did.
	s.send(`{"t":"finale","seq":10,"finale":{"outcome":"won","boss":"Goblin King","hp":0,"max_hp":1500,"raiders":3,` +
		`"time":240000,"answers":90,"right":80,"damage":1500,"dodged":2,"attacks":1}}`)
	s.send(`{"t":"summary","summary":{"answers":30,"right":28,"damage":500,"dodged":0,"attacks":1}}`)
	st = waitFor(t, p, "the summary", func(s PlayState) bool { return s.Summary != nil })
	if st.Raid != nil || st.Finale == nil || st.Finale.Outcome != RaidWon || st.Finale.Time != 4*time.Minute ||
		st.Finale.Right != 80 || st.Summary.Damage != 500 {
		t.Fatalf("finale %+v, summary %+v, raid %+v", st.Finale, st.Summary, st.Raid)
	}

	// Another raid is a new one, and clears the finale.
	s.send(strings.Replace(raidMsg, `"seq":5`, `"seq":11`, 1))
	st = waitFor(t, p, "the second raid", func(s PlayState) bool { return s.Raid != nil })
	if st.Raid.Number != 2 || st.Finale != nil || st.Summary != nil {
		t.Fatalf("second raid %+v, finale %+v", st.Raid, st.Finale)
	}
}

func TestPlayRaidRejoinKeepsTheRaid(t *testing.T) {
	f, c, conns := playing(t)
	s := joinRoom(t, f, c, conns)
	p := c.Play()
	s.send(raidMsg)
	s.send(`{"t":"word","word":{"n":1,"prompt":"the dog"}}`)
	waitFor(t, p, "the raid", func(s PlayState) bool { return s.Raid != nil && s.Raid.Word != nil })
	s.close(1006, "")

	s2 := next(t, conns)
	s2.expect("join")
	s2.send(`{"t":"room","seq":9,"room":{"code":"ABCDEF","mode":"raid","you":"m2","ends_at":1790000000000,` +
		`"members":[{"id":"m1","role":"host"},{"id":"m2","role":"learner","name":"Brave Otter"}],` +
		`"raid":{"boss":"Goblin King","seed":7,"lang":"fr","hp":800,"max_hp":1000,"ends_in":500000,"raiders":2,` +
		`"word":{"n":4,"prompt":"the cat"}}}}`)
	st := waitFor(t, p, "back in the raid", func(s PlayState) bool { return s.Phase == PlayInRoom && s.Raid != nil && s.Raid.HP == 800 })
	if st.Raid.Number != 1 || st.Room.Mode != ModeRaid || st.Raid.Word == nil || st.Raid.Word.N != 4 {
		t.Fatalf("raid %+v in %q", st.Raid, st.Room.Mode)
	}
	if !p.Answer(4, "le chat", false) {
		t.Fatal("couldn't answer after the rejoin")
	}
	s2.expect("answer")
}
