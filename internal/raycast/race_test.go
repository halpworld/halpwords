//go:build race

package raycast

// raceEnabled is true under the race detector, which skews timings.
const raceEnabled = true
