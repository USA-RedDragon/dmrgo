// Package growth holds functions that take a slice dst and return it. The
// Grow functions allocate only by growing dst, which the dst growth rule of
// the gate must prove. The Bad functions can grow another slice, or do
// something that the gate cannot follow, so the rule must reject them.
package growth

import "strconv"

var (
	global []int
	hook   = func(s []int) []int { return append(s, 1) }
)

// GrowAppend appends n to dst.
func GrowAppend(dst []int, n int) []int {
	return append(dst, n)
}

// GrowHelper appends through a helper that returns the slice and a pointer
// to the new element.
func GrowHelper(dst []int) []int {
	var p *int
	dst, p = appendOne(dst)
	*p = 1
	return dst
}

//go:noinline
func appendOne(out []int) ([]int, *int) {
	if len(out) < cap(out) {
		out = out[:len(out)+1]
	} else {
		out = append(out, 0)
	}
	return out, &out[len(out)-1]
}

// GrowLocal appends to a local that holds a slice of dst.
func GrowLocal(dst []int, n int) []int {
	d := dst[:len(dst)]
	for i := range n {
		d = append(d, i)
	}
	return d
}

// GrowChain passes dst through two levels of helpers.
func GrowChain(dst []int) []int {
	return twice(dst)
}

func twice(s []int) []int {
	s, _ = appendOne(s)
	s, _ = appendOne(s)
	return s
}

// BadNamedResult returns dst through a named result. The gate does not
// follow which value a named result holds at a return, and a named result
// starts as nil, so it rejects the function.
func BadNamedResult(dst []int) (out []int, n int) {
	out = append(dst, 1)
	return out, len(out)
}

// BadLocal grows a local slice that starts empty.
func BadLocal(dst []int) []int {
	var s []int
	for i := range len(dst) + 1 {
		s = append(s, i)
	}
	return append(dst, s...)
}

// BadGlobal grows a package-level slice.
func BadGlobal(dst []int) []int {
	global = append(global, 1)
	return dst
}

// BadHelperArg passes a helper a slice that is not dst.
func BadHelperArg(dst []int) []int {
	global, _ = appendOne(global)
	return dst
}

// BadFixed passes a helper a slice of a fixed array, which it grows when
// the array is full.
func BadFixed(dst []int) []int {
	var a [2]int
	s := a[:0]
	for range 3 {
		s, _ = appendOne(s)
	}
	return append(dst, s...)
}

// BadReturn returns a slice that is not dst.
func BadReturn(dst []int) []int {
	return append(global, dst...)
}

// BadFuncValue grows dst through a func value.
func BadFuncValue(dst []int) []int {
	return hook(dst)
}

// BadStdlib grows dst in a stdlib function, whose source the gate does not
// check.
func BadStdlib(dst []byte) []byte {
	return strconv.AppendInt(dst, 1, 10)
}

// BadCallee calls a function that grows a package-level slice.
func BadCallee(dst []int) []int {
	addGlobal()
	return append(dst, 1)
}

func addGlobal() {
	global = append(global, 1)
}

// BadName names its slice out, not dst.
func BadName(out []int) []int {
	return append(out, 1)
}

// GrowInPlace grows dst in a func literal that it calls in place, as cgo
// writes a C call, and the literal returns on its own.
func GrowInPlace(dst []int) []int {
	n := func() int {
		dst = append(dst, 1)
		return len(dst)
	}()
	return dst[:n]
}
