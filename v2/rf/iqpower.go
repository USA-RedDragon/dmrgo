package rf

const (
	iqPowerBlocks = 256
	iqPowerUs     = 500
	iqPowerEdge   = 4
)

type iqPower struct {
	blk  []float32
	acc  float32
	k    int
	per  int
	n    int64
	rate float64
	lag  float64
}

// newIQPower keeps the power over span seconds behind the input, and at
// least iqPowerBlocks blocks. Span covers the replay lookback plus the
// samples buffered ahead of the discriminator, so a replayed slot gets a
// power measurement as a live one does.
func newIQPower(hz int, lag, span float64) *iqPower {
	n := max(iqPowerBlocks, int((span+lag)*1e6/iqPowerUs)+iqPowerEdge)
	return &iqPower{blk: make([]float32, n), per: max(hz*iqPowerUs/1000000, 1), rate: float64(hz), lag: lag}
}

func (p *iqPower) push(y complex64) {
	p.acc += real(y)*real(y) + imag(y)*imag(y)
	if p.k++; p.k == p.per {
		p.blk[p.n%int64(len(p.blk))] = p.acc / float32(p.per)
		p.n++
		p.acc, p.k = 0, 0
	}
}

func (p *iqPower) mean(t0, t1 float64) (float32, bool) {
	sec := float64(p.per) / p.rate
	b0, b1 := int64((t0+p.lag)/sec), int64((t1+p.lag)/sec)
	n := int64(len(p.blk))
	if b0 < 0 || b1 < b0 || b1 >= p.n || p.n-b0 > n {
		return 0, false
	}
	var s float32
	for b := b0; b <= b1; b++ {
		s += p.blk[b%n]
	}
	return s / float32(b1-b0+1), true
}
