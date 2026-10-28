package resample

import "testing"

func TestAllocsPush(t *testing.T) {
	h := table(32, 64, true, 0.45)
	var c Complex
	var r Real
	c.Init(h, 32, 64, true, 44100.0/48000, make([]complex64, 64))
	r.Init(h, 32, 64, true, 44100.0/48000, make([]int16, 32))
	bc, br := make([]complex64, 4), make([]int16, 4)
	if n := testing.AllocsPerRun(1000, func() {
		c.Push(1, bc)
		r.Push(1, br)
	}); n != 0 {
		t.Errorf("%v allocations per Push", n)
	}
}
