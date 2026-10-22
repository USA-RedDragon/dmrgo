//go:build !tinygo

package bs_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestAllocs(t *testing.T) {
	in := merge(call(t, 1, 9, 3191868, 3, enums.Slot1, rf, 0), timed(loadBin(t, "d-sms.bin"), enums.Slot2, bs.Source{Peer: 3}, 0))
	in = merge(in, call(t, 1, 91, 3120000, 3, enums.Slot1, bs.Source{Peer: 2}, 30*time.Millisecond))
	for _, mode := range []bs.Mode{bs.ModeRF, bs.ModeNetwork} {
		r := newRepeater(t, &bs.Config{Mode: mode, ColorCode: 1, CACH: true})
		out := make([]bs.Output, 0, bs.MaxOutputs)
		allocs := testing.AllocsPerRun(3, func() {
			r.Reset()
			j := 0
			for now := time.Duration(0); now < 8*time.Second; now += 10 * time.Millisecond {
				for ; j < len(in) && in[j].at <= now; j++ {
					out = r.Inbound(in[j].at, &in[j].raw, layer2.Hint{Slot: in[j].slot}, in[j].from, out[:0])
				}
				if at, ok := r.Deadline(); ok && at <= now {
					out = r.Tick(now, out[:0])
				}
			}
		})
		if allocs != 0 {
			t.Errorf("%v: %v allocations", mode, allocs)
		}
	}
	p := newPool(t)
	for i, data := range append(seeds(), corpusFiles(t, "FuzzRepeater")...) {
		s := parseScript(data, p)
		r := newRepeater(t, &s.cfg)
		out := make([]bs.Output, 0, bs.MaxOutputs)
		dec := layer2.NewBurst()
		allocs := testing.AllocsPerRun(3, func() {
			r.Reset()
			for k := range s.ops {
				o := &s.ops[k]
				switch o.kind {
				case 0, 1:
					out = r.Inbound(o.at, &o.raw, o.h, o.from, out[:0])
				case 2:
					out = r.Decoded(o.at, decoded(dec, &o.raw), o.h, o.from, out[:0])
				case 3:
					out = r.Tick(o.at, out[:0])
				case 4:
					out = r.Lost(o.at, o.slot, o.from, out[:0])
				case 5:
					_ = r.Reject(o.slot)
				}
			}
		})
		if allocs != 0 {
			t.Errorf("seed %d: %v allocations", i, allocs)
		}
	}
}
