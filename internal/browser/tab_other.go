//go:build !js

package browser

// HoldTab reports true: outside a web browser the game is one process
// with its own saves, and there are no tabs to share them.
func HoldTab() bool { return true }
