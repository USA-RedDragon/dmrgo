// Package zeroalloc holds the allocation tests of the library. Its test
// files list the exported functions and methods with a call that
// testing.AllocsPerRun measures, and TestAllocs fails when a call allocates
// more than its entry allows. TestAllocsCoverAPI fails for an exported
// function or method that has no entry and no exemption. The tests do not
// build with TinyGo, whose testing.AllocsPerRun does not count allocations.
package zeroalloc
