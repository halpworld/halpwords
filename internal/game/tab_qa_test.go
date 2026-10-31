package game

import (
	"sync/atomic"
	"testing"
	"time"
)

// Tries of the lock never overlap, however many updates come while one is
// slow: a slow answer from the browser must not pile up goroutines.
func TestQAStarterNeverOverlapsTries(t *testing.T) {
	useTempDir(t)
	var running, peak, calls atomic.Int32
	release := make(chan struct{})
	hold := func() bool {
		n := running.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		if calls.Add(1) > 1 {
			<-release // every try after the first is slow
		}
		running.Add(-1)
		return false
	}
	s, err := start(func(*Context) Scene { return nil }, hold, func(func(*Context) Scene) (*Game, error) { return &Game{ctx: &Context{}}, nil })
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10*holdEvery; i++ {
		if err := s.Update(); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(20 * time.Millisecond)
	if peak.Load() > 1 {
		t.Errorf("%d tries at once", peak.Load())
	}
	if c := calls.Load(); c != 2 { // the first at start, one more still waiting
		t.Errorf("%d tries; a slow one should be waited for, not repeated", c)
	}
	close(release)
}

// Once started, the Starter is the game: Update passes on, Waiting is
// false, and the lock was tried once.
func TestQAStarterHeldAtOnceAndLayout(t *testing.T) {
	useTempDir(t)
	var tries atomic.Int32
	s, err := start(func(*Context) Scene { return nil }, func() bool { tries.Add(1); return true },
		func(func(*Context) Scene) (*Game, error) { return &Game{ctx: &Context{}}, nil })
	if err != nil {
		t.Fatal(err)
	}
	if s.Waiting() || tries.Load() != 1 {
		t.Fatalf("waiting %v tries %d", s.Waiting(), tries.Load())
	}
	if w, h := s.Layout(1, 1); w != ScreenW || h != ScreenH {
		t.Errorf("layout %dx%d", w, h)
	}
	s2, _ := start(func(*Context) Scene { return nil }, func() bool { return false }, New)
	if w, h := s2.Layout(10, 10); w != ScreenW || h != ScreenH {
		t.Errorf("waiting layout %dx%d", w, h)
	}
}
