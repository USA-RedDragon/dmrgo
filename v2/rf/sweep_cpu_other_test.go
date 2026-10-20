//go:build !linux

package rf

import "time"

func threadCPUSeconds(f func()) float64 {
	t := time.Now()
	f()
	return time.Since(t).Seconds()
}
