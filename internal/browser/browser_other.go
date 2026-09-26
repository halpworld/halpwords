//go:build !js

package browser

import (
	"os/exec"
	"runtime"
)

// InBrowser reports whether the game runs in a web browser.
const InBrowser = false

// Here is the web game's address. Outside a browser there is none.
func Here() string { return "" }

// open asks the system to open url in the player's browser, without
// waiting for it.
func open(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
