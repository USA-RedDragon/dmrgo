//go:build !linux && !tinygo

package fsktx

import "time"

func threadCPU(f func()) float64 {
	start := time.Now()
	f()
	return float64(time.Since(start).Nanoseconds())
}
