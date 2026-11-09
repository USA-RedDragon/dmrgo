package rf

import (
	"os"
	"strconv"
	"sync"
	"testing"
)

type carrierCount struct {
	slots, wrong, keyUp, keyUpWrong, held, heldWrong int
}

func (a *carrierCount) add(b carrierCount) {
	a.slots, a.wrong, a.keyUp, a.keyUpWrong = a.slots+b.slots, a.wrong+b.wrong, a.keyUp+b.keyUp, a.keyUpWrong+b.keyUpWrong
	a.held, a.heldWrong = a.held+b.held, a.heldWrong+b.heldWrong
}

// carrierOn gives the carrier of the burst of a slot whose burst starts at t
// seconds into the signal of envelope env: on, off, or neither when the
// carrier keys up or down within it.
func carrierOn(env []float64, t float64) (on, known bool) {
	k0, k1 := int((t+burstSeconds/8)*basebandRate), int((t+7*burstSeconds/8)*basebandRate)
	var s float64
	for i := k0; i < k1; i++ {
		if i >= 0 && i < len(env) {
			s += env[i]
		}
	}
	s /= float64(k1 - k0)
	return s > 0.5, s < 0.05 || s > 0.95
}

// carrierRun receives an MS stream of 4 direct mode calls with 0.3 s gaps at
// cn dB at Tier 3 and checks the carrier measure of each slot (noteCarrier)
// against the envelope on the air. A key-up case is a carrier slot after a
// dark slot of its parity, which the measure gets right when it reads the dark
// slot dark and the slot lit; a held case is a carrier slot after a carrier
// slot of its parity, right when it reads both lit.
func carrierRun(t testing.TB, run uint64, cn float64) carrierCount {
	cfg := directConfig()
	cfg.Source += uint32(run * 7919)
	cfg.Dest += uint32(run % 97)
	f, e, _ := directCallsOf(t, cfg, 4)
	x := noisyFM(t, f, e, 1400, cn, 9000+run)
	r := mustNew(RxConfig{Rate: basebandRate, Tier: Tier3, Format: IQFormatCU8})
	c := r.core()
	soft, _ := c.ext.(*softExt)
	var n carrierCount
	type mark struct{ on, lit bool }
	seen := map[int32]mark{}
	stream := c.streams
	last := int32(-1)
	for k := 0; k < len(x); k += 480 {
		if _, err := r.writeBB(x[k:min(k+480, len(x))]); err != nil {
			t.Fatal(err)
		}
		if !c.locked() || c.streams != stream {
			stream, last = c.streams, -1
			clear(seen)
			if !c.locked() {
				continue
			}
		}
		for j := max(last+1, c.next-2); j < c.next; j++ {
			base := c.first + int64(j)*slotSymbols + int64(c.ms.off[j&1])
			ticks, _, _ := c.burstTicks(base)
			on, known := carrierOn(e, tickSeconds(ticks)-benchPad)
			if !known || soft.car.noted[j&1] != j {
				continue
			}
			lit := soft.car.dark[j&1] == 0
			seen[j] = mark{on, lit}
			n.slots++
			if lit != on {
				n.wrong++
				pw, _, _ := c.slotPower(base, false)
				t.Logf("run %d slot %d at %.4f s: carrier %v, read lit %v, power %.4g, floor %.4g, carrier level %.4g", run, j, tickSeconds(ticks)-benchPad, on, lit, pw, soft.car.pLo, soft.car.pHi)
			}
			p, ok := seen[j-2]
			if !on || !ok {
				continue
			}
			if p.on {
				n.held++
				if !p.lit || !lit {
					n.heldWrong++
				}
			} else {
				n.keyUp++
				if p.lit || !lit {
					n.keyUpWrong++
				}
			}
		}
		last = c.next - 1
	}
	return n
}

// TestCarrierPresentWeak checks that the carrier measure of the lean core
// tells a new key-up after a gap from a held weak stream at 1 dB C/N: no case
// of either kind is misread. The slots of a stream that locks before any
// full carrier slot, on the noise or the ramp of a call, have no carrier to
// split from and read lit; the log counts every misread slot.
// DMRGO_CARRIER_RUNS sets the count of streams (default 4).
func TestCarrierPresentWeak(t *testing.T) {
	t.Parallel()
	runs := 4
	if v, err := strconv.Atoi(os.Getenv("DMRGO_CARRIER_RUNS")); err == nil && v > 0 {
		runs = v
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	var sum carrierCount
	sem := make(chan struct{}, 8)
	for run := range uint64(runs) {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			n := carrierRun(t, run, 1)
			<-sem
			mu.Lock()
			sum.add(n)
			mu.Unlock()
		}()
	}
	wg.Wait()
	t.Logf("%d runs: %d slots, %d misread; %d key-up cases, %d misread; %d held cases, %d misread", runs, sum.slots, sum.wrong, sum.keyUp, sum.keyUpWrong, sum.held, sum.heldWrong)
	if sum.keyUpWrong != 0 || sum.heldWrong != 0 || sum.keyUp == 0 || sum.held == 0 {
		t.Errorf("%d of %d key-up and %d of %d held cases misread", sum.keyUpWrong, sum.keyUp, sum.heldWrong, sum.held)
	}
}
