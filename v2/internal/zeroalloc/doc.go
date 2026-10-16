// Package zeroalloc holds the TinyGo allocation test of the library.
// TinyGo's testing.AllocsPerRun does not count allocations, so
// TestTinyGoAllocs reads runtime.MemStats around the main decode and encode
// calls and fails for a call that makes heap garbage.
package zeroalloc
