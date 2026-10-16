//go:build !linux

package rf

import "time"

// threadCPUSeconds returns the wall time of f where the thread CPU time is
// not available.
func threadCPUSeconds(f func()) float64 {
	t := time.Now()
	f()
	return time.Since(t).Seconds()
}
