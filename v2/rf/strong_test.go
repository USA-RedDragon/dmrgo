package rf

import (
	"runtime"
	"sync"
	"testing"
)

// strongCase is a stream of TestStrongSignalExact: its input mode, its
// stream, and the seeds of the noise that once lost or broke a burst.
type strongCase struct {
	mode  string
	ms    bool
	seeds []uint64
}

// TestStrongSignalExact checks that Tier 0 recovers every burst of a stream
// at a C/N of 30 dB bit exact, among them the first burst after the carrier
// comes up, which the symbol timing can sample about half a symbol off, and
// the first burst of a transmission of an MS on the slots of the lock of
// the one before, whose SYNC can end between two of its symbols.
func TestStrongSignalExact(t *testing.T) {
	t.Parallel()
	const cn = 30
	bsSlots, bsTruth := benchStream(t, 2)
	freq, env, msTruth := msStream(t, 2, 0.37)
	cases := []strongCase{
		{mode: benchAudio, seeds: []uint64{101, 103, 104, 108, 128, 147, 162}},
		{mode: benchAudio, ms: true, seeds: []uint64{113, 118, 128, 149}},
		{mode: benchIQ, seeds: []uint64{111, 112, 128, 138, 152}},
		{mode: benchIQ, ms: true, seeds: []uint64{100, 106, 111, 112, 115, 120, 124, 126, 131, 133, 145, 148, 158, 161}},
	}
	type result struct {
		c    strongCase
		seed uint64
		sc   map[string]*benchScore
	}
	var mu sync.Mutex
	var res []result
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	for _, c := range cases {
		for _, seed := range c.seeds {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				var x []complex64
				if c.ms {
					x = noisyFM(freq, env, 1400, cn, seed)
				} else {
					x = noisyBaseband(bsSlots, 1400, cn, seed)
				}
				var got []Burst
				if c.mode == benchAudio {
					got = receiveAudioTier(t, discAudio(x, benchAudioHz), Tier0)
				} else {
					got = receiveBaseband(x, Tier0)
				}
				var sc map[string]*benchScore
				if c.ms {
					sc = scoreMS(msTruth, got)
				} else {
					sc = scoreBench(bsTruth, got)
				}
				mu.Lock()
				res = append(res, result{c, seed, sc})
				mu.Unlock()
			}()
		}
	}
	wg.Wait()
	for _, r := range res {
		for _, p := range benchPayloads() {
			// The scorer counts the SYNC of BS-sourced bursts only.
			if s := r.sc[p]; s.Exact != s.Truth && (p != "sync" || !r.c.ms) {
				t.Errorf("%s ms %v seed %d: %s %d of %d exact", r.c.mode, r.c.ms, r.seed, p, s.Exact, s.Truth)
			}
		}
	}
}
