//go:build !tinygo

package packet_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
)

// TestAllocsAppendBursts segments a packet at each rate into a local Packet
// and appends its bursts. The Packet must stay off the heap.
func TestAllocsAppendBursts(t *testing.T) {
	b := packet.NewBuilder(127)
	data := payload(100)
	dst := make([]layer2.RawBurst, 0, 128)
	for _, rate := range []enums.DataType{enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeRate1} {
		n := testing.AllocsPerRun(20, func() {
			p, err := b.Segment(confirmed(), data, rate)
			if err != nil {
				t.Fatal(err)
			}
			if dst, err = packet.AppendBursts(dst[:0], &p, 1, layer2.SyncSourceMS); err != nil {
				t.Fatal(err)
			}
		})
		if n != 0 {
			t.Errorf("%s: %.1f allocations per packet, want 0", rate, n)
		}
	}
}
