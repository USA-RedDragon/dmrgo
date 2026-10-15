package zeroalloc_test

import (
	"slices"
	"testing"
)

// entry is one measured call of an exported function or method.
type entry struct {
	// api is the package path below the module root and the function, or
	// the type and method, as in "layer2.Burst.DecodeFromBytes".
	api string
	// name tells the cases of one api apart.
	name string
	// allocs is the number of allocations the call may make.
	allocs float64
	// run makes the call. Setup and warm-up happen before, outside run.
	run func()
}

// entries returns the entries of every package.
func entries(t testing.TB) []entry {
	t.Helper()
	return slices.Concat(
		layer2Entries(t),
		packetEntries(t),
		framerEntries(t),
		vocoderEntries(t),
		bitEntries(t),
		enumsEntries(t),
		fecEntries(t),
		layer3Entries(t),
		pduEntries(t),
	)
}

// TestAllocs checks the allocations of every entry.
func TestAllocs(t *testing.T) {
	for _, e := range entries(t) {
		if n := testing.AllocsPerRun(100, e.run); n != e.allocs {
			t.Errorf("%s (%s): %.1f allocs per call, want %.0f", e.api, e.name, n, e.allocs)
		}
	}
}

// BenchmarkAllocs runs every entry as a sub-benchmark.
func BenchmarkAllocs(b *testing.B) {
	for _, e := range entries(b) {
		b.Run(e.api+"/"+e.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				e.run()
			}
		})
	}
}
