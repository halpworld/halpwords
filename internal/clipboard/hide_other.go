//go:build !windows && !js

package clipboard

import "os/exec"

func hide(*exec.Cmd) {}
