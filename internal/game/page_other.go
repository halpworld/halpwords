//go:build !js

package game

import "github.com/halpworld/halpwords/internal/link"

// watchPage does nothing outside a web browser: the game saves the queue
// when it quits.
func watchPage(func() *link.Client) {}
