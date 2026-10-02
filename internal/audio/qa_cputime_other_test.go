//go:build !unix

package audio

import "time"

var cpuStart = time.Now()

// cpuNow falls back to the wall clock where there is no getrusage.
func cpuNow() time.Duration { return time.Since(cpuStart) }
