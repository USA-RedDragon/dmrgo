package csbk

import (
	"math"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/internal/pcg"
)

const pcgStream = 721347520444481703

func seedPCG(seed uint64) pcg.Source {
	return pcg.NewStream(seed, pcgStream)
}

func holdoff(p *pcg.Source, maxDur time.Duration) time.Duration {
	if maxDur <= 0 {
		return 0
	}
	v := int64(p.Uint32()&0x7FFFFFFF)<<32 | int64(p.Uint32())
	m := int64(maxDur)
	if m < math.MaxInt64 {
		m++
	}
	return time.Duration(v % m)
}
