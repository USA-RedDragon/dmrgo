package ambe

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

func checkCases[C any](t *testing.T, n int, gen func(i int) C, check func(c *C) string) {
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
