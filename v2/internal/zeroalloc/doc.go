// Package zeroalloc holds the allocation tests of the library. Its test
// files list the exported functions and methods with a call that
// testing.AllocsPerRun measures, and TestAllocs fails when a call makes a
// different number of allocations than its entry states. TestAllocsCoverAPI fails for an exported
// function or method that has no entry and no exemption. These tests do not
// build with TinyGo, whose testing.AllocsPerRun does not count allocations.
// Under TinyGo, TestTinyGoAllocs reads runtime.MemStats around the main
// decode and encode calls instead, and fails for a call that makes heap
// garbage.
package zeroalloc
