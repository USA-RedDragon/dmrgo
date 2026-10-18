//go:build !linux && !tinygo

package fsktx

import "time"

// threadCPU returns the wall time while f runs, in nanoseconds, where the
// CPU time of a thread is not available.
func threadCPU(f func()) float64 {
	start := time.Now()
	f()
	return float64(time.Since(start).Nanoseconds())
}
