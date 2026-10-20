package trunk

type pcg struct {
	state uint64
	inc   uint64
}

func (p *pcg) seed(seed uint64) {
	p.state, p.inc = 0, seed<<1|1
	p.next()
	p.state += seed
	p.next()
}

func (p *pcg) next() uint32 {
	old := p.state
	p.state = old*6364136223846793005 + p.inc
	xorshifted := uint32((((old >> 18) ^ old) >> 27) & 0xFFFFFFFF)
	rot := uint32(old >> 59)
	return xorshifted>>rot | xorshifted<<((-rot)&31)
}

func (p *pcg) between(n int) int {
	if n <= 1 {
		return 1
	}
	return 1 + int(uint64(p.next())*uint64(n)>>32)
}
