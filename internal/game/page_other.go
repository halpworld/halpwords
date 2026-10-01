//go:build !js

package game

import "github.com/halpworld/halpwords/internal/link"

// watchPage does nothing outside a web browser: the game saves the queue
// when it quits, and the window's close button is handled in Update.
func watchPage(func() *link.Client, func()) {}
