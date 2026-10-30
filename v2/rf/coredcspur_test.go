//go:build !tinygo

package rf

import (
	"math"
	"testing"
)

func TestCoreTier3LockOffsetDCSpur(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 1)
	for _, off := range []float64{1400, -1400} {
		for _, cn := range []float64{10, 20} {
			iq := cu8Spur(noisyBaseband(slots, off, cn, 7), 32, -0.5)
			r, err := newReceiver(RxConfig{Rate: basebandRate, Tier: Tier3, Format: IQFormatCU8}, ptr(coreIQ3Options()))
			if err != nil {
				t.Fatal(err)
			}
			h := r.host().iqd
			step := 2 * basebandRate / 100
			var bs []Burst
			at, lockAt := 0.0, -1
			for i := 0; i+step <= len(iq) && lockAt < 0; i += step {
				bs = r.AppendBursts(bs[:0], iq[i:i+step])
				if h.d.locked {
					lockAt, at = i, float64(h.conv.hz)
				}
			}
			switch {
			case lockAt < 0:
				t.Errorf("core Tier 3 at %.0f Hz, C/N %.0f dB, DC spur: no lock", off, cn)
			case math.Abs(at-off) > iqAFCPullHz:
				t.Errorf("core Tier 3 at %.0f Hz, C/N %.0f dB, DC spur: oscillator at %.0f Hz at the lock, want within %d Hz", off, cn, at, iqAFCPullHz)
			}
		}
	}
}
