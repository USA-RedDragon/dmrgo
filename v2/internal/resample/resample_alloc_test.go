package resample

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/allocstest"
)

func TestAllocsPush(t *testing.T) {
	h := table(32, 64, true, 0.45)
	var c Complex
	var r Real
	c.Init(h, 32, 64, true, 44100.0/48000, make([]complex64, 64))
	r.Init(h, 32, 64, true, 44100.0/48000, make([]int16, 32))
	var p Poly32
	p.Init(make([]float32, 30), 3, 10, 1, make([]float32, 6))
	bc, br, bp := make([]complex64, 4), make([]int16, 4), make([]float64, 10)
	if n := allocstest.PerRun(t, 1000, func() {
		c.Push(1, bc)
		r.Push(1, br)
		p.Push(1, bp)
	}); n != 0 {
		t.Errorf("%v allocations per Push", n)
	}
}
