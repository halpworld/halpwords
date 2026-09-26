//go:build !js

package link

import (
	"testing"
	"time"

	"github.com/halpworld/halpwords/pkg/race"
)

const raceMsg = `{"t":"race","seq":5,"race":{"seed":12345,"goal":3,"list":"title: Race\nlanguage: fr\nthe dog = le chien\n","starts_in":0,"ends_in":600000,` +
	`"racers":[{"id":"m2","floor":1,"monsters":0,"x":0,"y":0,"status":"racing"},{"id":"m3","floor":1,"monsters":0,"x":0,"y":0,"status":"racing"}]}}`

func TestPlayRace(t *testing.T) {
	f, c, conns := playing(t)
	s := joinRoom(t, f, c, conns)
	p := c.Play()
	if p.Report(race.Report{Floor: 1}) {
		t.Fatal("reported with no race")
	}
	s.send(raceMsg)
	st := waitFor(t, p, "the race", func(s PlayState) bool { return s.Race != nil })
	if r := st.Race; r.Seed != 12345 || r.Goal != 3 || r.Number != 1 || len(r.Racers) != 2 || r.List == "" {
		t.Fatalf("race %+v", r)
	}

	// Progress goes, but only when it changes.
	if !p.Report(race.Report{Floor: 1, X: 4, Y: 5}) {
		t.Fatal("the first report didn't go")
	}
	if m := s.expect("progress"); m["floor"] != float64(1) || m["x"] != float64(4) || m["y"] != float64(5) || m["fell"] != nil {
		t.Fatalf("sent %v", m)
	}
	if p.Report(race.Report{Floor: 1, X: 4, Y: 5}) {
		t.Fatal("the same report went again")
	}
	if !p.Report(race.Report{Floor: 2, X: 1, Y: 1}) {
		t.Fatal("a new floor didn't go at once")
	}
	s.expect("progress")

	// Others' progress moves their markers.
	s.send(`{"t":"progress","seq":6,"id":"m3","progress":{"floor":2,"monsters":1,"x":7,"y":8,"status":"racing"}}`)
	waitFor(t, p, "m3's progress", func(s PlayState) bool {
		r, ok := s.Race.Racer("m3")
		return ok && r.Floor == 2 && r.X == 7 && r.Monsters == 1
	})

	// An event the game doesn't know still counts in the sequence, so the
	// results after it need no resync.
	s.send(`{"t":"cheer","seq":7,"id":"m3"}`)
	s.send(`{"t":"results","seq":8,"results":[{"id":"m3","name":"Quiet Fox","place":1,"floor":3,"monsters":2,"time":95000,"status":"finished"},` +
		`{"id":"m2","name":"Brave Otter","place":2,"floor":2,"monsters":0,"status":"racing"}]}`)
	st = waitFor(t, p, "the results", func(s PlayState) bool { return s.Race == nil && len(s.Results) == 2 })
	if r := st.Results[0]; r.Name != "Quiet Fox" || r.Place != 1 || r.Time != 95*time.Second || r.Status != RacerFinished {
		t.Fatalf("results %+v", st.Results)
	}
	if p.Report(race.Report{Floor: 2, X: 2, Y: 1}) {
		t.Fatal("reported after the race")
	}
	if !p.Say("thanks") {
		t.Fatal("couldn't say thanks")
	}
	s.expect("say") // not a join to resync

	// A second race is a new one.
	s.send(`{"t":"race","seq":9,"race":{"seed":999,"goal":3,"list":"x","starts_in":60000,"ends_in":660000,"racers":[{"id":"m2","floor":1,"monsters":0,"x":0,"y":0,"status":"racing"}]}}`)
	st = waitFor(t, p, "the second race", func(s PlayState) bool { return s.Race != nil })
	if st.Race.Number != 2 || len(st.Results) != 0 || time.Until(st.Race.StartsAt) < 50*time.Second {
		t.Fatalf("second race %+v, results %v", st.Race, st.Results)
	}
	if p.Report(race.Report{Floor: 1}) {
		t.Fatal("reported before the start")
	}
}

func TestPlayRaceRejoinKeepsTheRace(t *testing.T) {
	f, c, conns := playing(t)
	s := joinRoom(t, f, c, conns)
	p := c.Play()
	s.send(raceMsg)
	waitFor(t, p, "the race", func(s PlayState) bool { return s.Race != nil })
	p.Report(race.Report{Floor: 1, X: 3, Y: 3})
	s.expect("progress")
	s.close(1006, "")

	s2 := next(t, conns)
	s2.expect("join")
	room := `{"t":"room","seq":9,"room":{"code":"ABCDEF","mode":"race","you":"m2","ends_at":1790000000000,` +
		`"members":[{"id":"m1","role":"host"},{"id":"m2","role":"learner","name":"Brave Otter"}],` +
		`"race":{"seed":12345,"goal":3,"list":"title: Race\nlanguage: fr\nthe dog = le chien\n","starts_in":0,"ends_in":500000,` +
		`"racers":[{"id":"m2","floor":1,"monsters":0,"x":3,"y":3,"status":"racing"}]}}}`
	s2.send(room)
	st := waitFor(t, p, "back in the race", func(s PlayState) bool { return s.Phase == PlayInRoom && s.Race != nil && len(s.Race.Racers) == 1 })
	if st.Race.Number != 1 || st.Room.Mode != ModeRace {
		t.Fatalf("race %+v in %q", st.Race, st.Room.Mode)
	}
	// The last report goes again, in case it was lost.
	if !p.Report(race.Report{Floor: 1, X: 3, Y: 3}) {
		t.Fatal("the report didn't go again after the rejoin")
	}
	s2.expect("progress")
}
