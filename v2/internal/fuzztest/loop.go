// Package fuzztest holds the fuzz and robustness helpers that the tests
// of more than one package share. Only tests import it.
package fuzztest

// Loop gives the repeat window of a script of n steps. The run plays
// steps 0 to hi-1, then steps lo to hi-1 reps more times, then the
// steps from hi to the end. total is the number of run steps, at most
// limit.
func Loop(n int, from, to, reps uint8, limit int) (lo, hi, total int) {
	lo = min(int(from), n)
	hi = max(min(int(to), n), lo)
	return lo, hi, min(n+int(reps)*(hi-lo), limit)
}

// Index gives the script step that run step s of the window from
// [Loop] plays.
func Index(s, lo, hi int, reps uint8) int {
	if s < hi {
		return s
	}
	s -= hi
	w := int(reps) * (hi - lo)
	if s < w {
		return lo + s%(hi-lo)
	}
	return hi + s - w
}

// Repeat gives the repeat of the window from [Loop] that run step s is
// in, from 1 to reps, or 0 when s is not in a repeat.
func Repeat(s, lo, hi int, reps uint8) int {
	if s < hi || hi == lo || s-hi >= int(reps)*(hi-lo) {
		return 0
	}
	return (s-hi)/(hi-lo) + 1
}
