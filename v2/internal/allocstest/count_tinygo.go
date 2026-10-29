//go:build tinygo

package allocstest

import (
	"runtime"
	"testing"
)

// PerRun returns testing.AllocsPerRun(runs, f). TinyGo's
// testing.AllocsPerRun does not count allocations; use Mallocs under
// TinyGo.
func PerRun(tb testing.TB, runs int, f func()) float64 {
	tb.Helper()
	return testing.AllocsPerRun(runs, f)
}

// Mallocs returns the number of heap allocations that runtime.MemStats
// counts during one call of f. It calls setup first if setup is not nil;
// the allocations of setup do not count. TinyGo has no heap profile, so
// under TinyGo the count includes the allocations of other goroutines.
func Mallocs(tb testing.TB, setup, f func()) uint64 {
	tb.Helper()
	m, _ := window(setup, f)
	return m
}

// Bytes returns the number of heap bytes that runtime.MemStats counts as
// allocated during one call of f. It calls setup first if setup is not nil;
// the allocations of setup do not count.
func Bytes(tb testing.TB, setup, f func()) uint64 {
	tb.Helper()
	_, b := window(setup, f)
	return b
}

func window(setup, f func()) (mallocs, total uint64) {
	if setup != nil {
		setup()
	}
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	f()
	runtime.ReadMemStats(&b)
	return b.Mallocs - a.Mallocs, b.TotalAlloc - a.TotalAlloc
}
