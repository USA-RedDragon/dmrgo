//go:build !tinygo

package bs_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func TestAllocsExpress(t *testing.T) {
	v := advNetCall(t, 3, 0, 1, 1, time.Second, 3191868)
	d := netData(t, loadBin(t, "d-sms.bin"), enums.Slot1, 0, 2, v[len(v)-1].at+20*time.Millisecond)
	d[1].at, d[2].at = d[2].at, d[1].at
	d[1], d[2] = d[2], d[1]
	in := append(append([]advPkt(nil), v...), d...)
	for _, mode := range []bs.Mode{bs.ModeRF, bs.ModeNetwork} {
		r := newRepeater(t, &bs.Config{Mode: mode, ColorCode: 1, CACH: true})
		out := make([]bs.Output, 0, bs.MaxOutputs)
		allocs := testing.AllocsPerRun(3, func() {
			r.Reset()
			j := 0
			for now := time.Duration(0); now < 4*time.Second; now += time.Millisecond {
				for ; j < len(in) && in[j].at <= now; j++ {
					out = r.Inbound(now, &in[j].raw, in[j].h, bs.Source{Peer: in[j].peer}, out[:0])
				}
				for range 8 {
					if at, ok := r.Deadline(); !ok || at > now {
						break
					}
					out = r.Tick(now, out[:0])
				}
			}
		})
		if allocs != 0 {
			t.Errorf("%v: %v allocations", mode, allocs)
		}
	}
}
