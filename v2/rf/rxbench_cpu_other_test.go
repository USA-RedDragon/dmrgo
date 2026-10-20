//go:build !unix

package rf

import "time"

func cpuSeconds(f func()) float64 {
	start := time.Now()
	f()
	return time.Since(start).Seconds()
}
