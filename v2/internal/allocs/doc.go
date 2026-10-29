// Package allocs holds tests that check with allocstest that the main
// paths of the library allocate nothing in steady state, on the captures in
// layer2/testdata. TinyGo's testing.AllocsPerRun does not count
// allocations, so under TinyGo TestTinyGoAllocs counts with
// allocstest.Mallocs and allocstest.Bytes around the main decode and encode
// calls instead.
package allocs
