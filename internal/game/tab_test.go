package game

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// A tab that doesn't hold the lock reads and writes nothing: it makes no
// game, tries the lock again about once a second, and starts the game
// once it gets it (halpwords#111). One that holds it, and a game outside
// a browser, start at once.
func TestStarterWaitsForTheOtherTab(t *testing.T) {
	useTempDir(t)
	first := func(*Context) Scene { return nil }
	var held atomic.Bool
	var tries, made atomic.Int32
	hold := func() bool { tries.Add(1); return held.Load() }
	mk := func(func(*Context) Scene) (*Game, error) { made.Add(1); return &Game{ctx: &Context{}}, nil }

	s, err := start(first, hold, mk)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Waiting() || made.Load() != 0 || tries.Load() != 1 {
		t.Fatalf("waiting %v, made %d, tries %d", s.Waiting(), made.Load(), tries.Load())
	}
	screen := ebiten.NewImage(ScreenW, ScreenH)
	// Not held: it keeps waiting, trying once every holdEvery updates.
	for i := 0; i < 3*holdEvery+holdEvery/2; i++ {
		if err := s.Update(); err != nil {
			t.Fatal(err)
		}
		waitTry(s)
		s.Draw(screen)
	}
	if !s.Waiting() || made.Load() != 0 {
		t.Fatal("started while the other tab holds the lock")
	}
	if n := tries.Load(); n < 3 || n > 5 {
		t.Errorf("%d tries in 3.5 seconds", n)
	}
	s.Close() // nothing to save, and no game to close
	// The other tab closes: the next try gets it, and the game starts.
	held.Store(true)
	for i := 0; i < 2*holdEvery && s.Waiting(); i++ {
		s.Update()
		waitTry(s)
	}
	if s.Waiting() || made.Load() != 1 {
		t.Fatalf("not started once the lock was free: waiting %v, made %d", s.Waiting(), made.Load())
	}

	// Held from the start: New at once, with no waiting screen.
	made.Store(0)
	s, err = start(first, func() bool { return true }, mk)
	if err != nil || s.Waiting() || made.Load() != 1 {
		t.Fatalf("held: %v waiting %v made %d", err, s.Waiting(), made.Load())
	}
	// New's error is Start's.
	boom := errors.New("boom")
	if _, err := start(first, func() bool { return true }, func(func(*Context) Scene) (*Game, error) { return nil, boom }); !errors.Is(err, boom) {
		t.Errorf("err %v", err)
	}
	// ...and, after waiting, Update's.
	held.Store(false)
	s, _ = start(first, hold, func(func(*Context) Scene) (*Game, error) { return nil, boom })
	held.Store(true)
	var uerr error
	for i := 0; i < 3*holdEvery && uerr == nil; i++ {
		uerr = s.Update()
		waitTry(s)
	}
	if !errors.Is(uerr, boom) {
		t.Errorf("update err %v", uerr)
	}
}

// waitTry waits for a try of the lock on its way, so the test doesn't
// race the goroutine; it puts the answer back for Update.
func waitTry(s *Starter) {
	if s.trying == nil {
		return
	}
	held := <-s.trying
	out := make(chan bool, 1)
	out <- held
	s.trying = out
}

// The waiting screen's lines fit the screen.
func TestTabLinesFit(t *testing.T) {
	useTempDir(t)
	s, err := start(func(*Context) Scene { return nil }, func() bool { return false }, New)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range tabLines {
		if w := s.font.Width(l, 1); w > ScreenW-32 {
			t.Errorf("%q is %dpx wide", l, w)
		}
	}
}
