//go:build unix

package rf

import "syscall"

// cpuSeconds returns the CPU time of the process while f runs alone.
func cpuSeconds(f func()) float64 {
	var a, b syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &a)
	f()
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &b)
	sec := func(t syscall.Timeval) float64 { return float64(t.Sec) + float64(t.Usec)/1e6 }
	return sec(b.Utime) + sec(b.Stime) - sec(a.Utime) - sec(a.Stime)
}
