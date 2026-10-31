//go:build !tinygo

package allocstest

import (
	"bytes"
	"context"
	"flag"
	"math"
	"reflect"
	"runtime"
	"runtime/metrics"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"testing"
)

var state struct {
	sync.Mutex
	objects, bytes uint64
	marks          uint64
	mark           *[16]byte
	created        []metrics.Sample
	tiny           []metrics.Sample
	nested         uint64
	skipped        uint64
	stale          bool
	nestedStacks   map[string]bool
}

// PerRun returns the average number of heap allocations of one call of f,
// as testing.AllocsPerRun(runs, f) does: it calls f once to warm up, then
// runs times, and divides the count by runs in integers. It counts only the
// allocations inside the calls of f on the calling goroutine. Like
// testing.AllocsPerRun, it panics in a parallel test. It fails tb if a
// goroutine starts during the measurement.
func PerRun(tb testing.TB, runs int, f func()) float64 {
	tb.Helper()
	objects, _ := measure(tb, f, runs, f, false)
	return math.Floor(float64(objects) / float64(runs))
}

// Mallocs returns the number of heap allocations inside one call of f on
// the calling goroutine, with GOMAXPROCS 1. It calls setup first if setup
// is not nil; the allocations of setup do not count. Use it for a call that
// cannot repeat without a new setup. Like testing.AllocsPerRun, it panics
// in a parallel test. It fails tb if a goroutine starts during the
// measurement.
func Mallocs(tb testing.TB, setup, f func()) uint64 {
	tb.Helper()
	objects, _ := measure(tb, f, 1, setup, false)
	return objects
}

// Bytes returns the number of heap bytes allocated inside one call of f on
// the calling goroutine, with GOMAXPROCS 1. It calls setup first if setup
// is not nil; the allocations of setup do not count. It fails tb if a
// goroutine starts during the call.
func Bytes(tb testing.TB, setup, f func()) uint64 {
	tb.Helper()
	_, n := measure(tb, f, 1, setup, true)
	return n
}

func measure(tb testing.TB, f func(), runs int, setup func(), wantBytes bool) (objects, allocated uint64) {
	tb.Helper()
	state.Lock()
	defer state.Unlock()
	if state.created == nil {
		state.created = []metrics.Sample{{Name: "/sched/goroutines-created:goroutines"}, {Name: "/sched/goroutines:goroutines"}}
		state.tiny = []metrics.Sample{{Name: "/gc/heap/tiny/allocs:objects"}}
	}
	state.skipped = 0
	if state.stale {
		state.objects, state.bytes, state.nested = settle(tb)
	}
	state.stale = true
	rate := runtime.MemProfileRate
	mallocs, total, tiny, started := window(f, runs, setup, wantBytes)
	runtime.MemProfileRate = rate
	if started != 0 {
		if n := labeled(tb); n != 0 || !fuzzing() {
			tb.Fatalf("allocstest: %d goroutines started during the measurement (%d still running with the label of the measured call); their allocations have no stack through the measured call", started, n)
		}
	}
	if mallocs == 0 && total == 0 {
		state.stale = false
		return 0, 0
	}
	o, b, n := settle(tb)
	state.stale = false
	state.skipped, state.nested = n-state.nested, n
	objects, allocated = o-state.objects+tiny, b-state.bytes
	state.objects, state.bytes = o, b
	return objects, allocated
}

func settle(tb testing.TB) (objects, allocated, nestedObjects uint64) {
	tb.Helper()
	rate := runtime.MemProfileRate
	runtime.MemProfileRate = 1
	mark()
	runtime.MemProfileRate = rate
	state.marks++
	for range 50 {
		runtime.GC()
		o, b, n, m := profiled(tb)
		if m < state.marks {
			continue
		}
		state.marks = m
		return o, b, n
	}
	tb.Fatal("allocstest: the heap profile does not show the sentinel allocation after 50 GC cycles")
	return 0, 0, 0
}

func fuzzing() bool {
	f := flag.Lookup("test.fuzz")
	return f != nil && f.Value.String() != ""
}

func labeled(tb testing.TB) int {
	tb.Helper()
	var buf bytes.Buffer
	if err := pprof.Lookup("goroutine").WriteTo(&buf, 1); err != nil {
		tb.Fatal(err)
	}
	n := 0
	for line := range strings.Lines(buf.String()) {
		if strings.HasPrefix(line, "# labels:") && strings.Contains(line, `"allocstest":"measured"`) {
			n++
		}
	}
	return n
}

//go:noinline
func mark() { state.mark = new([16]byte) }

func window(f func(), runs int, setup func(), wantBytes bool) (mallocs, total, tiny, started uint64) {
	created, system := goroutines()
	var tiny0 uint64
	warm := true
	run := func() {
		if warm {
			warm = false
			if setup != nil {
				setup()
			}
			return
		}
		tiny0 = tinyAllocs()
		runtime.MemProfileRate = 1
		for range runs {
			inside(f)
		}
	}
	pprof.SetGoroutineLabels(pprof.WithLabels(context.Background(), pprof.Labels("allocstest", "measured")))
	if wantBytes {
		mallocs, total = bracket(run)
	} else {
		mallocs = uint64(testing.AllocsPerRun(1, run))
	}
	pprof.SetGoroutineLabels(context.Background())
	tiny = tinyAllocs() - tiny0
	created2, system2 := goroutines()
	return mallocs, total, tiny, created2 - created - min(created2-created, max(system2, system)-system)
}

func tinyAllocs() uint64 {
	metrics.Read(state.tiny)
	if state.tiny[0].Value.Kind() != metrics.KindUint64 {
		return 0
	}
	return state.tiny[0].Value.Uint64()
}

func bracket(run func()) (mallocs, total uint64) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	run()
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	run()
	runtime.ReadMemStats(&b)
	return b.Mallocs - a.Mallocs, b.TotalAlloc - a.TotalAlloc
}

func goroutines() (created, system uint64) {
	metrics.Read(state.created)
	if state.created[0].Value.Kind() != metrics.KindUint64 {
		return 0, 0
	}
	all := state.created[1].Value.Uint64()
	var user uint64
	if n := runtime.NumGoroutine(); n > 0 {
		user = uint64(n)
	}
	return state.created[0].Value.Uint64(), all - min(all, user)
}

//go:noinline
func inside(f func()) { f() }

func profiled(tb testing.TB) (objects, allocated, nestedObjects, marks uint64) {
	tb.Helper()
	name := "\t" + runtime.FuncForPC(reflect.ValueOf(inside).Pointer()).Name() + "+"
	markName := "\t" + runtime.FuncForPC(reflect.ValueOf(mark).Pointer()).Name() + "+"
	var buf bytes.Buffer
	if err := pprof.Lookup("heap").WriteTo(&buf, 1); err != nil {
		tb.Fatal(err)
	}
	var o, b uint64
	var pcs string
	counted := true
	for line := range strings.Lines(buf.String()) {
		switch {
		case strings.HasPrefix(line, "#"):
			if counted {
				break
			}
			if strings.Contains(line, name) {
				counted = true
				if nested(pcs) {
					nestedObjects += o
					break
				}
				objects += o
				allocated += b
			} else if strings.Contains(line, markName) {
				marks += o
				counted = true
			}
		case strings.Contains(line, " @ ") && !strings.HasPrefix(line, "heap profile:"):
			var ok bool
			if o, b, ok = allocCounts(line); !ok {
				tb.Fatalf("allocstest: heap profile record without counts: %q", line)
			}
			_, pcs, _ = strings.Cut(line, " @ ")
			counted = false
		}
	}
	return objects, allocated, nestedObjects, marks
}

func nested(pcs string) bool {
	if v, ok := state.nestedStacks[pcs]; ok {
		return v
	}
	var stack []uintptr
	for _, f := range strings.Fields(pcs) {
		if pc, err := strconv.ParseUint(strings.TrimPrefix(f, "0x"), 16, 64); err == nil {
			stack = append(stack, uintptr(pc))
		}
	}
	found, other := false, false
	frames := runtime.CallersFrames(stack)
	for more := true; more && !found; {
		var frame runtime.Frame
		frame, more = frames.Next()
		if allocator(frame.Function) {
			found = other
		} else {
			other = true
		}
	}
	if state.nestedStacks == nil {
		state.nestedStacks = map[string]bool{}
	}
	state.nestedStacks[pcs] = found
	return found
}

func allocator(fn string) bool {
	return strings.HasPrefix(fn, "runtime.mallocgc") || fn == "runtime.profilealloc" || fn == "runtime.mProf_Malloc"
}

func allocCounts(rec string) (objects, allocated uint64, ok bool) {
	i := strings.IndexByte(rec, '[')
	j := strings.IndexByte(rec, ']')
	if i < 0 || j < i {
		return 0, 0, false
	}
	o, b, ok := strings.Cut(rec[i+1:j], ":")
	if !ok {
		return 0, 0, false
	}
	objects, err1 := strconv.ParseUint(strings.TrimSpace(o), 10, 64)
	allocated, err2 := strconv.ParseUint(strings.TrimSpace(b), 10, 64)
	return objects, allocated, err1 == nil && err2 == nil
}
