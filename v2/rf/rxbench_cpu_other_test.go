//go:build !unix

package rf

import "time"

// cpuSeconds returns the wall time while f runs, where the CPU time of the
// process is not available.
func cpuSeconds(f func()) float64 {
	start := time.Now()
	f()
	return time.Since(start).Seconds()
}
