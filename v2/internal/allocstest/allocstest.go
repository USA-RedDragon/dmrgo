// Package allocstest counts the heap allocations of a call in tests. It
// counts only the allocations that the measured goroutine makes inside the
// call.
//
// testing.AllocsPerRun and a runtime.MemStats delta read the malloc counter
// of the whole process, so each allocation that another goroutine makes
// during the measurement counts against the measured call. Both run the call
// with GOMAXPROCS 1: when the scheduler preempts the measured goroutine,
// the other goroutine runs on the same P inside the window. Preemption
// occurs more often when the machine is short of CPU.
//
// The functions of this package read the same process-wide counter first.
// When it shows no allocation, the call made none. When it shows
// allocations, the functions count them again from the heap profile, which
// they record at MemProfileRate 1 during the call: they keep only the
// allocations with a stack that passes through the call. The heap profile
// does not record a tiny allocation that the runtime packs into an existing
// block, so the functions add each such allocation of the window, from the
// /gc/heap/tiny/allocs:objects metric, as one of the call. They measure the
// call once and do not repeat it, so an allocation on a path that the call
// takes only sometimes is counted when it occurs. They read the heap
// profile only after it shows a sentinel allocation made after the call.
// The runtime publishes a record one or two GC cycles after the allocation,
// in cycle order, so the sentinel's record comes after the records of the
// call.
//
// An allocation of the call can start a GC, and the runtime can then
// allocate for itself inside that allocation, for example a sudog when
// gcStart waits on a semaphore, or the mark workers that gcStart starts
// when GOMAXPROCS grew. Its stack passes through the call, but it also
// passes through runtime.mallocgc of the outer allocation, so the functions
// do not count it. The allocation that started it still counts.
//
// When a measurement stops before it reads the heap profile, for example
// because the call panics or starts a goroutine, the next measurement reads
// the heap profile first, so the allocations of the stopped call do not
// count against the next call.
//
// The allocations of a goroutine that the call starts have no stack through
// the call, so the functions fail the test when a user goroutine starts
// during the measurement (from Go 1.26, which has the
// /sched/goroutines-created:goroutines metric). While fuzzing, the fuzzing
// engine starts goroutines of its own, so then they fail the test only for a
// goroutine that still runs with the pprof label that they set on the
// measured goroutine.
//
// The forbidigo rule in .golangci.yaml rejects testing.AllocsPerRun and
// runtime.ReadMemStats outside this package.
package allocstest
