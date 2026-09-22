// Package input collects keyboard input once per tick.
package input

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// State holds this tick's typed characters.
type State struct {
	Chars []rune
}

// Update reads the characters typed since the last tick.
func (s *State) Update() {
	s.Chars = ebiten.AppendInputChars(s.Chars[:0])
}

// Pressed reports whether any of keys was pressed this tick.
func Pressed(keys ...ebiten.Key) bool {
	for _, k := range keys {
		if inpututil.IsKeyJustPressed(k) {
			return true
		}
	}
	return false
}

// Repeat reports whether key was pressed this tick or is held and repeating.
func Repeat(key ebiten.Key) bool {
	const delay, interval = 24, 3
	d := inpututil.KeyPressDuration(key)
	return d == 1 || (d >= delay && (d-delay)%interval == 0)
}

// Confirm reports whether Enter was pressed.
func Confirm() bool { return Pressed(ebiten.KeyEnter, ebiten.KeyNumpadEnter) }

// Back reports whether Escape was pressed.
func Back() bool { return Pressed(ebiten.KeyEscape) }

// Up and Down are for menus: arrow keys or W/S.
func Up() bool   { return Repeat(ebiten.KeyArrowUp) || Repeat(ebiten.KeyW) }
func Down() bool { return Repeat(ebiten.KeyArrowDown) || Repeat(ebiten.KeyS) }
