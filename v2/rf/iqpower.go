package rf

const (
	iqPowerBlocks = 256
	iqPowerUs     = 500
)

type iqPower struct {
	blk  [iqPowerBlocks]float32
	acc  float32
	k    int
	per  int
	n    int64
	rate float64
	lag  float64
}

func newIQPower(hz int, lag float64) *iqPower {
	return &iqPower{per: max(hz*iqPowerUs/1000000, 1), rate: float64(hz), lag: lag}
}

func (p *iqPower) push(y complex64) {
	p.acc += real(y)*real(y) + imag(y)*imag(y)
	if p.k++; p.k == p.per {
		p.blk[p.n%iqPowerBlocks] = p.acc / float32(p.per)
		p.n++
		p.acc, p.k = 0, 0
	}
}

func (p *iqPower) mean(t0, t1 float64) (float32, bool) {
	sec := float64(p.per) / p.rate
	b0, b1 := int64((t0+p.lag)/sec), int64((t1+p.lag)/sec)
	if b0 < 0 || b1 < b0 || b1 >= p.n || p.n-b0 > iqPowerBlocks {
		return 0, false
	}
	var s float32
	for b := b0; b <= b1; b++ {
		s += p.blk[b%iqPowerBlocks]
	}
	return s / float32(b1-b0+1), true
}
