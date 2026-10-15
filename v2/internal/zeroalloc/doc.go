// Package zeroalloc holds the allocation tests of the library. Its test
// files list the exported functions and methods with a call that
// testing.AllocsPerRun measures, and TestAllocs fails when a call allocates
// more than its entry allows. The tests do not build with TinyGo, whose
// testing.AllocsPerRun does not count allocations.
package zeroalloc
