package dvsi_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/dvsisim"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi"
)

func TestAllocsDecodeReport(t *testing.T) {
	pairs := dvsisim.Pairs(2)
	c, err := dvsi.New(dvsisim.New(pairs, dvsisim.Options{}), nil)
	if err != nil {
		t.Fatal(err)
	}
	pcm := make([]int16, 0, dvsi.FrameSamples)
	n := 0
	allocs := testing.AllocsPerRun(50, func() {
		pcm, _, _ = c.Decode(pcm[:0], pairs[n%2].Slot)
		pcm, _, _ = c.DecodeLost(pcm[:0])
		n++
	})
	if allocs != 0 {
		t.Errorf("Decode and DecodeLost: %.1f allocations", allocs)
	}
}
