package gfx

import "testing"

func TestSparksBurnOut(t *testing.T) {
	s := NewSparks(1)
	s.Burst(100, 100, Burst{N: 20, Speed: 3, Fall: 0.2, Life: 30})
	if !s.Active() || len(s.ps) != 20 {
		t.Fatalf("%d sparks after a burst of 20", len(s.ps))
	}
	for i := 0; i < 30; i++ {
		s.Update()
	}
	if s.Active() {
		t.Fatalf("%d sparks still flying after their life", len(s.ps))
	}
}

func TestNilSparksDoNothing(t *testing.T) {
	var s *Sparks
	s.Burst(0, 0, Burst{N: 5, Life: 10})
	s.Update()
	s.Draw(nil)
	if s.Active() {
		t.Fatal("nil sparks are active")
	}
}
