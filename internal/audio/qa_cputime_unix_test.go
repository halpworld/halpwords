//go:build unix

package audio

import (
	"syscall"
	"time"
)

// cpuNow is the CPU time this process has used. Unlike the wall clock it
// does not grow while the process waits for a core on a busy machine.
func cpuNow() time.Duration {
	var ru syscall.Rusage
	if syscall.Getrusage(syscall.RUSAGE_SELF, &ru) != nil {
		return 0
	}
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}
