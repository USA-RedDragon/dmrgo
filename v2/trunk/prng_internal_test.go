package trunk

import (
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/pcg"
)

type oldPCG struct {
	state uint64
	inc   uint64
}

func (p *oldPCG) seed(seed uint64) {
	p.state, p.inc = 0, seed<<1|1
	p.next()
	p.state += seed
	p.next()
}

func (p *oldPCG) next() uint32 {
	old := p.state
	p.state = old*6364136223846793005 + p.inc
	xorshifted := uint32((((old >> 18) ^ old) >> 27) & 0xFFFFFFFF)
	rot := uint32(old >> 59)
	return xorshifted>>rot | xorshifted<<((-rot)&31)
}

func (p *oldPCG) between(n int) int {
	if n <= 1 {
		return 1
	}
	return 1 + int(uint64(p.next())*uint64(n)>>32)
}

func TestBetweenMatchesOldGenerator(t *testing.T) {
	t.Parallel()
	for _, seed := range []uint64{0, 1, 42, 3191868, math.MaxUint64} {
		var old oldPCG
		old.seed(seed)
		cur := pcg.New(seed)
		for i := range 4000 {
			n := []int{0, 1, 2, 7, 16, 255, 1 << 20}[i%7]
			if a, b := old.between(n), between(&cur, n); a != b {
				t.Fatalf("seed %d, value %d: %d, old generator %d", seed, i, b, a)
			}
		}
	}
}
