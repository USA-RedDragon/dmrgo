// Package testmode tells the tests of the module how much work to do.
//
// By default a test runs a sample that still checks its claim. With
// DMRGO_LONG=1 it runs its full sample; the weekly long test job of CI
// sets it. Under the race detector a test that only computes numbers in
// one goroutine runs a smaller sample: the runs without the race detector
// check its statistics, and the race run only needs its code paths.
package testmode

import "os"

// Race reports whether the race detector is on.
const Race = race

// Long reports whether DMRGO_LONG asks for the full runs.
func Long() bool { return os.Getenv("DMRGO_LONG") == "1" }

// N returns long under Long, short under the race detector, and def
// otherwise.
func N(def, long, short int) int {
	switch {
	case Long():
		return long
	case Race:
		return short
	}
	return def
}
