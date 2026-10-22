package hytera_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/hytera"
)

func TestAllocsHytera(t *testing.T) {
	seeds := fuzzSeeds()
	buf := make([]byte, 0, 128)
	n := testing.AllocsPerRun(100, func() {
		for _, p := range seeds {
			if f, err := hytera.DecodeFrame(p); err == nil {
				buf, _ = f.AppendBinary(buf[:0])
				h := f.Hint(0)
				_ = f.SetHint(h)
				_ = f.HasBurst()
			}
			if h, err := hytera.DecodeHeartbeat(p); err == nil {
				buf, _ = h.AppendBinary(buf[:0])
			}
			if c, err := hytera.DecodeP2P(p); err == nil {
				buf, _ = c.AppendBinary(buf[:0])
				_, _ = c.RedirectPort()
				_, _ = c.Service()
			}
		}
	})
	if n != 0 {
		t.Fatalf("%v allocations", n)
	}
}
