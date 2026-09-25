//go:build !js

package clipboard

import (
	"context"
	"os/exec"
	"runtime"
	"time"
)

// commands are the paste commands to try on each system, in order.
func commands() [][]string {
	switch runtime.GOOS {
	case "darwin":
		return [][]string{{"pbpaste"}}
	case "windows":
		return [][]string{{"powershell", "-NoProfile", "-NonInteractive", "-Command", "Get-Clipboard"}}
	}
	return [][]string{
		{"wl-paste", "--no-newline"},
		{"xclip", "-selection", "clipboard", "-o"},
		{"xsel", "--clipboard", "--output"},
	}
}

// Read returns the text on the clipboard.
func Read() (string, error) {
	for _, c := range commands() {
		if _, err := exec.LookPath(c[0]); err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, c[0], c[1:]...)
		hide(cmd)
		out, err := cmd.Output()
		cancel()
		if err == nil {
			return string(out), nil
		}
	}
	return "", ErrNoClipboard
}
