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
	for k := range stale {
		if !ebiten.IsKeyPressed(k) {
			delete(stale, k)
		}
	}
}

// stale holds keys that seem held but may not be. macOS can lose the key
// release while the window switches full screen, and the key then looks
// held forever, repeating until the app is killed.
var stale = map[ebiten.Key]bool{}

// ForgetHeld ignores the keys held now until they are released. Call it
// when the window switches full screen.
func ForgetHeld() {
	for _, k := range inpututil.AppendPressedKeys(nil) {
		stale[k] = true
	}
}

// Held reports whether key is down.
func Held(key ebiten.Key) bool { return !stale[key] && ebiten.IsKeyPressed(key) }

// Duration reports for how many ticks key has been held, or 0.
func Duration(key ebiten.Key) int {
	if stale[key] {
		return 0
	}
	return inpututil.KeyPressDuration(key)
}

// Pressed reports whether any of keys was pressed this tick.
func Pressed(keys ...ebiten.Key) bool {
	for _, k := range keys {
		if Duration(k) == 1 {
			return true
		}
	}
	return false
}

// Repeat reports whether key was pressed this tick or is held and repeating.
func Repeat(key ebiten.Key) bool {
	const delay, interval = 24, 3
	d := Duration(key)
	return d == 1 || (d >= delay && (d-delay)%interval == 0)
}

// Confirm reports whether Enter was pressed.
func Confirm() bool { return Pressed(ebiten.KeyEnter, ebiten.KeyNumpadEnter) }

// Back reports whether Escape was pressed.
func Back() bool { return Pressed(ebiten.KeyEscape) }

// Up and Down are for menus: arrow keys or W/S.
func Up() bool   { return Repeat(ebiten.KeyArrowUp) || Repeat(ebiten.KeyW) }
func Down() bool { return Repeat(ebiten.KeyArrowDown) || Repeat(ebiten.KeyS) }
