// Package cases holds functions whose heap allocations the gate must find,
// named Alloc*, and functions that do not allocate, named NoAlloc*. Each
// Alloc function allocates in a way that -gcflags=-m does not report in its
// own body.
package cases

import (
	"errors"
	"strconv"
	"sync"
)

var (
	table = map[string]int{}
	sink  any
	mu    sync.Mutex
)

// errSentinel is a sentinel error.
var errSentinel = errors.New("sentinel")

// AllocMapWrite writes a map, which can grow it.
func AllocMapWrite(k string) { table[k] = 1 }

// AllocItoa calls strconv.Itoa, which allocates the string.
func AllocItoa(n int) string { return strconv.Itoa(n) }

// AllocConvert stores an int in an interface, which runtime.convT64 boxes.
func AllocConvert(n int) { sink = n }

// AllocConcat concatenates strings.
func AllocConcat(a, b string) string { return a + b }

//go:noinline
func grow(s []int) []int { return append(s, 1) }

// AllocCalleeAppend calls a function that grows a slice.
func AllocCalleeAppend() int { return len(grow(nil)) }

// AllocClosure returns a closure that captures n.
func AllocClosure(n int) func() int { return func() int { return n } }

type boxer interface{ box(n int) any }

type intBoxer struct{}

func (intBoxer) box(n int) any { return n }

// AllocInterfaceCall calls an interface method whose only implementation
// allocates.
func AllocInterfaceCall(b boxer, n int) { sink = b.box(n) }

// NewBoxer returns a boxer.
func NewBoxer() boxer { return intBoxer{} }

var hook = func(n int) any { return n }

// AllocFuncValue calls a func value that allocates.
func AllocFuncValue(n int) { sink = hook(n) }

// AllocDefer defers a closure that allocates.
func AllocDefer(n int) {
	defer func() { sink = n }()
}

// NoAllocSum adds the values of s.
func NoAllocSum(s []int) int {
	t := 0
	for _, v := range s {
		t += v
	}
	return t
}

// NoAllocMapRead reads a map.
func NoAllocMapRead(k string) int { return table[k] }

// NoAllocPanic panics with a value it builds, which only a panic path does.
func NoAllocPanic(n int) {
	if n < 0 {
		panic("negative: " + strconv.Itoa(n))
	}
}

// NoAllocSentinel returns a sentinel error.
func NoAllocSentinel() error { return errSentinel }

// NoAllocLock locks and unlocks a mutex. A contended Lock blocks the goroutine
// in runtime.semacquire1, which allocates a sudog only when its caches are
// empty, which the runtime cache rule allows.
func NoAllocLock() int {
	mu.Lock()
	n := len(table)
	mu.Unlock()
	return n
}

// NoAllocTypeSwitch switches on an interface type. The runtime builds the
// cache of the switch once for each dynamic type, which the runtime cache
// rule allows.
func NoAllocTypeSwitch(v any) int {
	switch v.(type) {
	case interface{ String() string }:
		return 1
	case error:
		return 2
	}
	return 0
}

// NoAllocArray fills an array on the stack.
func NoAllocArray() [64]byte {
	var a [64]byte
	for i := range a {
		a[i] = byte(i)
	}
	return a
}

// AllocLoopPanic allocates on every pass of a loop that leaves only by a
// panic.
func AllocLoopPanic(n int) {
	for {
		if n < 0 {
			panic("negative")
		}
		grow(nil)
		n--
	}
}

//go:noinline
func noop() {}

// AllocGo starts a goroutine, which allocates a g when the free list is
// empty.
func AllocGo() { go noop() }
