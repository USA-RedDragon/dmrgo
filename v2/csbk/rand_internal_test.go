package csbk

import (
	"math"
	"testing"
	"time"
)

type oldPCG struct {
	state uint64
}

func oldSeed(seed uint64) oldPCG {
	var p oldPCG
	p.next()
	p.state += seed
	p.next()
	return p
}

func (p *oldPCG) next() uint32 {
	old := p.state
	p.state = old*6364136223846793005 + 1442695040888963407
	x := uint32((old>>18 ^ old) >> 27 & 0xFFFFFFFF)
	rot := uint32(old >> 59 & 31)
	return x>>rot | x<<((32-rot)&31)
}

func (p *oldPCG) duration(maxDur time.Duration) time.Duration {
	if maxDur <= 0 {
		return 0
	}
	v := int64(p.next()&0x7FFFFFFF)<<32 | int64(p.next())
	m := int64(maxDur)
	if m < math.MaxInt64 {
		m++
	}
	return time.Duration(v % m)
}

func TestHoldoffMatchesOldGenerator(t *testing.T) {
	t.Parallel()
	for _, seed := range []uint64{0, 1, 42, 3191868, math.MaxUint64} {
		old, cur := oldSeed(seed), seedPCG(seed)
		for i := range 4000 {
			d := []time.Duration{0, 1, 2 * time.Second, 540 * time.Millisecond, math.MaxInt64}[i%5]
			if a, b := old.duration(d), holdoff(&cur, d); a != b {
				t.Fatalf("seed %d, value %d: %v, old generator %v", seed, i, b, a)
			}
		}
	}
}
