package testmode

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

// Cases checks n cases on GOMAXPROCS goroutines. It calls gen for each
// index from 0 to n-1 in order on the goroutine of t, so a gen that draws
// from one random source makes the same cases as a serial loop. check
// returns an empty string for a case that passes and the failure message
// otherwise. Cases fails t with the message of the first failing case in
// index order.
func Cases[C any](t testing.TB, n int, gen func(i int) C, check func(c *C) string) {
	t.Helper()
	const chunk, block = 1 << 14, 64
	workers := runtime.GOMAXPROCS(0)
	cases := make([]C, 0, chunk)
	msgs := make([]string, chunk)
	for base := 0; base < n; base += chunk {
		m := min(chunk, n-base)
		cases = cases[:0]
		for i := range m {
			cases = append(cases, gen(base+i))
		}
		var next atomic.Int64
		var wg sync.WaitGroup
		for range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					lo := int(next.Add(block)) - block
					if lo >= m {
						return
					}
					for i := lo; i < min(lo+block, m); i++ {
						msgs[i] = check(&cases[i])
					}
				}
			}()
		}
		wg.Wait()
		for _, msg := range msgs[:m] {
			if msg != "" {
				t.Fatal(msg)
			}
		}
	}
}
