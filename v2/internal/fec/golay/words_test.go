package golay

import (
	"runtime"
	"sync"
	"testing"
)

func everyWord(t *testing.T, n uint32, check func(w uint32) string) {
	t.Helper()
	parts := uint32(runtime.GOMAXPROCS(0))
	msgs := make([]string, parts)
	var wg sync.WaitGroup
	for p := range parts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for w := n / parts * p; w < n/parts*(p+1) || p == parts-1 && w < n; w++ {
				if msg := check(w); msg != "" {
					msgs[p] = msg
					return
				}
			}
		}()
	}
	wg.Wait()
	for _, msg := range msgs {
		if msg != "" {
			t.Fatal(msg)
		}
	}
}
