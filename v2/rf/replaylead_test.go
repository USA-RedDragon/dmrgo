//go:build !tinygo

package rf

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
)

func TestRxNoBurstsInNoiseLead(t *testing.T) {
	t.Parallel()
	type rx struct {
		name string
		run  func(x []complex64) []Burst
	}
	rxs := []rx{
		{"I/Q Tier 2", func(x []complex64) []Burst { return receiveBaseband(x, Tier2) }},
		{"I/Q Tier 3", func(x []complex64) []Burst { return receiveBaseband(x, Tier3) }},
		{"core I/Q Tier 3", func(x []complex64) []Burst { return receiveCore3(t, x) }},
		{"audio Tier 2", func(x []complex64) []Burst { return receiveAudioTier(t, discAudio(x, benchAudioHz), Tier2) }},
	}
	const seeds = 16
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	bad := map[string][]string{}
	for seed := range seeds {
		s := uint64(9100 + seed)
		x := callMix(t, 25, s, 0, callTalker{src: 3191868, superframes: 2, header: true, term: true}).x
		for _, r := range rxs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				n := 0
				for _, b := range r.run(x) {
					if b.Seconds < benchPad-0.01 {
						n++
					}
				}
				if n > 0 {
					mu.Lock()
					bad[r.name] = append(bad[r.name], fmt.Sprintf("seed %d: %d", s, n))
					mu.Unlock()
				}
			}()
		}
	}
	wg.Wait()
	for _, r := range rxs {
		if len(bad[r.name]) > 0 {
			t.Errorf("%s: bursts in the %.1f s of noise before the call, %v", r.name, benchPad, bad[r.name])
		}
	}
}
