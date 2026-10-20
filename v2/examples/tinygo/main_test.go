package main

import (
	"runtime"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func TestRun(t *testing.T) {
	t.Parallel()

	var b layer2.Burst
	f := newFollower()
	got := run(&b, f, capture)
	want := summary{bursts: 15, voice: 13, data: 2, reencoded: 15, target: 9990, source: 3191868, calls: 1, relayed: 20, ended: voice.EndTerminator}
	if got != want {
		t.Fatalf("run = %+v, want %+v", got, want)
	}

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	run(&b, f, capture)
	runtime.ReadMemStats(&after)
	if garbage := after.TotalAlloc - before.TotalAlloc; garbage != 0 {
		t.Errorf("run allocates %d bytes, want 0", garbage)
	}
}

func BenchmarkRun(b *testing.B) {
	var burst layer2.Burst
	f := newFollower()
	b.ReportAllocs()
	for b.Loop() {
		run(&burst, f, capture)
	}
}
