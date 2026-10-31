//go:build !tinygo

package allocstest

import (
	"fmt"
	"runtime"
	"runtime/metrics"
	"strings"
	"testing"
)

var sink, noisySink, otherSink any

var tiny *uint32

func busy() {
	x := 0
	for i := range 20_000_000 {
		x += i
	}
	sink = x == -1
}

func noisy(t *testing.T) {
	t.Helper()
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				noisySink = make([]byte, 64)
				runtime.Gosched()
			}
		}
	}()
	t.Cleanup(func() {
		close(stop)
		<-done
	})
}

//go:noinline
func deep(n int) {
	if n == 0 {
		sink = new([64]byte)
		return
	}
	deep(n - 1)
}

func TestAllocsOtherGoroutine(t *testing.T) {
	ask, reply := make(chan struct{}), make(chan struct{})
	go func() {
		for range ask {
			otherSink = make([]byte, 1<<20)
			reply <- struct{}{}
		}
	}()
	t.Cleanup(func() { close(ask) })
	other := func() {
		ask <- struct{}{}
		<-reply
	}
	rate := runtime.MemProfileRate
	state.created = []metrics.Sample{{Name: "/sched/goroutines-created:goroutines"}, {Name: "/sched/goroutines:goroutines"}}
	state.tiny = []metrics.Sample{{Name: "/gc/heap/tiny/allocs:objects"}}
	_, total, _, _, _ := window(other, 1, other, true)
	runtime.MemProfileRate = rate
	if total < 1<<20 {
		t.Fatalf("the process-wide counter shows %d bytes, not the 1 MiB of the other goroutine", total)
	}
	if n := Bytes(t, nil, other); n >= 1<<20 {
		t.Errorf("Bytes counts %d bytes, with the 1 MiB of another goroutine", n)
	}
	noisy(t)
	if n := PerRun(t, 3, busy); n != 0 {
		t.Errorf("PerRun counts %v allocations of a preempting goroutine", n)
	}
	if n := Mallocs(t, nil, busy); n != 0 {
		t.Errorf("Mallocs counts %d allocations of a preempting goroutine", n)
	}
}

func TestAllocsOwn(t *testing.T) {
	noisy(t)
	if n := PerRun(t, 3, func() { busy(); sink = new([64]byte) }); n != 1 {
		t.Errorf("PerRun = %v, want 1", n)
	}
	if n := Mallocs(t, nil, func() { deep(100) }); n != 1 {
		t.Errorf("Mallocs 100 frames down = %d, want 1", n)
	}
	if n := Bytes(t, nil, func() { sink = new([64]byte) }); n != 64 {
		t.Errorf("Bytes = %d, want 64", n)
	}
}

func TestAllocsTiny(t *testing.T) {
	if n := Mallocs(t, nil, func() {
		for range 8 {
			tiny = new(uint32)
		}
	}); n != 8 {
		t.Errorf("Mallocs of 8 tiny allocations = %d", n)
	}
	if n := PerRun(t, 10, func() { tiny = new(uint32) }); n != 1 {
		t.Errorf("PerRun of 1 tiny allocation = %v", n)
	}
}

func TestAllocsSometimes(t *testing.T) {
	calls := 0
	f := func() {
		calls++
		if calls%3 == 0 {
			sink = new([64]byte)
		}
	}
	for i := range 12 {
		want := uint64(0)
		if (calls+1)%3 == 0 {
			want = 1
		}
		if n := Mallocs(t, nil, f); n != want {
			t.Fatalf("call %d: Mallocs = %d, want %d", i, n, want)
		}
	}
}

func TestAllocsSetup(t *testing.T) {
	var p *[64]byte
	n := Mallocs(t, func() { p = new([64]byte) }, func() { p[0] = 1 })
	if n != 0 {
		t.Errorf("Mallocs counts %d allocations of setup", n)
	}
}

type fatalTB struct {
	testing.TB
	msg string
}

func (f *fatalTB) Helper() {}

func (f *fatalTB) Fatalf(format string, args ...any) {
	f.msg = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

func TestAllocsGoroutineStart(t *testing.T) {
	probe := []metrics.Sample{{Name: "/sched/goroutines-created:goroutines"}}
	if metrics.Read(probe); probe[0].Value.Kind() != metrics.KindUint64 {
		t.Skip("this Go has no /sched/goroutines-created:goroutines metric")
	}
	done := make(chan struct{})
	tb := &fatalTB{TB: t}
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		Mallocs(tb, nil, func() { go close(done) })
	}()
	<-finished
	<-done
	if !strings.Contains(tb.msg, "goroutines started") {
		t.Errorf("a call that starts a goroutine passes: %q", tb.msg)
	}
	if n := Mallocs(t, nil, func() { sink = new([64]byte) }); n != 1 {
		t.Errorf("Mallocs after a failed measurement = %d, want 1", n)
	}
}

var (
	big   []byte
	procs int
)

func TestAllocsNestedRuntime(t *testing.T) {
	procs = max(procs, runtime.GOMAXPROCS(0))
	setup := func(n int) func() {
		return func() {
			big = nil
			runtime.GC()
			runtime.GOMAXPROCS(n)
		}
	}
	procs++
	n := procs
	got := Mallocs(t, setup(n), func() { big = make([]byte, 16<<20) })
	if state.skipped == 0 {
		t.Error("the runtime made no allocation inside the allocation of the call")
	}
	if got != 1 {
		t.Errorf("Mallocs = %d with %d allocations that the runtime made inside the allocation of the call, want 1", got, state.skipped)
	}
	procs++
	n = procs
	b := Bytes(t, setup(n), func() { big = make([]byte, 16<<20) })
	if state.skipped == 0 {
		t.Error("the runtime made no allocation inside the allocation of the call")
	}
	if b != 16<<20 {
		t.Errorf("Bytes = %d with %d allocations that the runtime made inside the allocation of the call, want %d", b, state.skipped, 16<<20)
	}
	big = nil
}

func exiting(n int) chan struct{} {
	stop, ready := make(chan struct{}), make(chan struct{})
	for range n {
		go func() {
			ready <- struct{}{}
			<-stop
		}()
	}
	for range n {
		<-ready
	}
	return stop
}

func TestAllocsGoroutineExits(t *testing.T) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(max(4, runtime.GOMAXPROCS(0))))
	for i := range 10 {
		procs = max(procs, runtime.GOMAXPROCS(0)) + 1
		n := procs
		stop := exiting(1000)
		close(stop)
		got := Mallocs(t, func() {
			big = nil
			runtime.GC()
			runtime.GOMAXPROCS(n)
		}, func() { big = make([]byte, 16<<20) })
		if got != 1 {
			t.Fatalf("round %d: Mallocs = %d while other goroutines exit, want 1", i, got)
		}
	}
	big = nil
}

func TestAllocsGoroutineStartAmongExits(t *testing.T) {
	probe := []metrics.Sample{{Name: "/sched/goroutines-created:goroutines"}}
	if metrics.Read(probe); probe[0].Value.Kind() != metrics.KindUint64 {
		t.Skip("this Go has no /sched/goroutines-created:goroutines metric")
	}
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(max(4, runtime.GOMAXPROCS(0))))
	for i := range 10 {
		stop := exiting(1000)
		done, finished := make(chan struct{}), make(chan struct{})
		tb := &fatalTB{TB: t}
		go func() {
			defer close(finished)
			Mallocs(tb, nil, func() {
				close(stop)
				go close(done)
			})
		}()
		<-finished
		<-done
		if !strings.HasPrefix(tb.msg, "allocstest: 1 goroutines started") {
			t.Fatalf("round %d: a call that starts 1 goroutine while other goroutines exit: %q", i, tb.msg)
		}
	}
}
