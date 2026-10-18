//go:build !tinygo

package fsktx

import (
	"runtime"
	"syscall"
	"unsafe"
)

// threadCPU returns the CPU time of the calling thread while f runs, in
// nanoseconds, from CLOCK_THREAD_CPUTIME_ID, which counts in nanoseconds
// where getrusage counts in scheduler ticks.
func threadCPU(f func()) float64 {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const clockThreadCPUTime = 3
	var a, b syscall.Timespec
	_, _, _ = syscall.Syscall(syscall.SYS_CLOCK_GETTIME, clockThreadCPUTime, uintptr(unsafe.Pointer(&a)), 0)
	f()
	_, _, _ = syscall.Syscall(syscall.SYS_CLOCK_GETTIME, clockThreadCPUTime, uintptr(unsafe.Pointer(&b)), 0)
	return float64(b.Nano() - a.Nano())
}
