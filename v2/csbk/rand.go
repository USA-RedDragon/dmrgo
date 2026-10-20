package csbk

import (
	"math"
	"time"
)

type pcg struct {
	state uint64
}

const (
	pcgMultiplier = 6364136223846793005
	pcgIncrement  = 1442695040888963407
)

func seedPCG(seed uint64) pcg {
	var p pcg
	p.next()
	p.state += seed
	p.next()
	return p
}

func (p *pcg) next() uint32 {
	old := p.state
	p.state = old*pcgMultiplier + pcgIncrement
	x := uint32((old>>18 ^ old) >> 27 & 0xFFFFFFFF)
	rot := uint32(old >> 59 & 31)
	return x>>rot | x<<((32-rot)&31)
}

func (p *pcg) duration(maxDur time.Duration) time.Duration {
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
