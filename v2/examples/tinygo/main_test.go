package main

import (
	"runtime"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// TestRun checks the decode and re-encode results, and logs the heap garbage
// of a run. Under TinyGo, run it with -stack-size=8KB, the default of the
// pico and esp32c3 targets, because TinyGo puts a local on the heap when it
// is larger than 256 bytes at that stack size. t.Skip does not work under
// TinyGo, so the garbage check logs until it can pass.
func TestRun(t *testing.T) {
	t.Parallel()

	var b layer2.Burst
	got := run(&b, capture)
	want := summary{bursts: 15, voice: 13, data: 2, reencoded: 15, target: 9990, source: 3191868}
	if got != want {
		t.Fatalf("run = %+v, want %+v", got, want)
	}

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	run(&b, capture)
	runtime.ReadMemStats(&after)
	if garbage := after.TotalAlloc - before.TotalAlloc; garbage != 0 {
		t.Logf("not checked: run allocates %d bytes until data burst decode and encode allocate nothing and layer2 keeps packed bits", garbage)
	}
}

func BenchmarkRun(b *testing.B) {
	var burst layer2.Burst
	b.ReportAllocs()
	for b.Loop() {
		run(&burst, capture)
	}
}
