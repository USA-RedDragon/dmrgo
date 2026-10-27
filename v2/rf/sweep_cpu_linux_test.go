//go:build !tinygo

package rf

import (
	"runtime"
	"syscall"
)

func threadCPUSeconds(f func()) float64 {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var a, b syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_THREAD, &a)
	f()
	_ = syscall.Getrusage(syscall.RUSAGE_THREAD, &b)
	sec := func(t syscall.Timeval) float64 { return float64(t.Sec) + float64(t.Usec)/1e6 }
	return sec(b.Utime) + sec(b.Stime) - sec(a.Utime) - sec(a.Stime)
}
