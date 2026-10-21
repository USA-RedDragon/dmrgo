package ipsc_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
)

func TestAllocsIPSC(t *testing.T) {
	seeds := fuzzSeeds()
	buf := make([]byte, 0, 128)
	n := testing.AllocsPerRun(100, func() {
		for _, p := range seeds {
			if u, err := ipsc.DecodeUser(p); err == nil {
				buf, _ = u.AppendBinary(buf[:0])
				h := u.Hint()
				if raw, err := u.RawBurst(7, layer2.SyncSourceBS); err == nil {
					_ = u.SetRawBurst(raw, h.Kind, h.VoiceBurst, h.DataType)
				}
			}
			if c, err := ipsc.DecodeControl(p); err == nil {
				buf, _ = c.AppendBinary(buf[:0])
			}
		}
	})
	if n != 0 {
		t.Fatalf("%v allocations", n)
	}
}
