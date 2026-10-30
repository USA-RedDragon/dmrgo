//go:build !tinygo

package rf

import (
	"math"
	"os"
	"testing"
)

func cu8Spur(x []complex64, scale, dc float32) []byte {
	b := make([]byte, 2*len(x))
	for i, v := range x {
		b[2*i] = byte(max(min(roundf(real(v)*scale+127.5+dc), 255), 0))
		b[2*i+1] = byte(max(min(roundf(imag(v)*scale+127.5+dc), 255), 0))
	}
	return b
}

func lockNCO(t *testing.T, iq []byte, rate int, tier Tier) (atLock, after float64, ok bool) {
	t.Helper()
	r := mustNew(RxConfig{Rate: rate, Tier: tier, Format: IQFormatCU8})
	h := r.host()
	step := 2 * rate / 100
	nco := func() (float64, bool) {
		if h.iqd != nil {
			return float64(h.iqd.conv.hz), h.iqd.d.locked
		}
		return h.e.chz.ncoHz, h.e.locked
	}
	var bs []Burst
	lockAt := -1
	for i := 0; i+step <= len(iq); i += step {
		bs = r.AppendBursts(bs[:0], iq[i:i+step])
		hz, locked := nco()
		switch {
		case locked && lockAt < 0:
			lockAt, atLock = i, hz
		case lockAt >= 0 && i-lockAt >= iqAfterLock*step:
			return atLock, hz, true
		}
	}
	return 0, 0, false
}

func TestIQLockOffsetDCSpur(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 1)
	for _, off := range []float64{1400, -1400} {
		for _, cn := range []float64{10, 20} {
			iq := cu8Spur(noisyBaseband(slots, off, cn, 7), 32, -0.5)
			for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
				at, after, ok := lockNCO(t, iq, basebandRate, tier)
				switch {
				case !ok:
					t.Errorf("tier %d at %.0f Hz, C/N %.0f dB, DC spur: no lock", tier-Tier0, off, cn)
				case (tier == Tier0 || tier == Tier3) && math.Abs(at-off) > iqAFCPullHz:
					t.Errorf("tier %d at %.0f Hz, C/N %.0f dB, DC spur: oscillator at %.0f Hz at the lock, want within %d Hz", tier-Tier0, off, cn, at, iqAFCPullHz)
				case math.Abs(after-off) > iqLockOffsetBound:
					t.Errorf("tier %d at %.0f Hz, C/N %.0f dB, DC spur: oscillator at %.0f Hz %d0 ms after the lock, want within %d Hz", tier-Tier0, off, cn, after, iqAfterLock, iqLockOffsetBound)
				}
			}
		}
	}
}

func TestIQLockOffsetXPR8400(t *testing.T) {
	t.Parallel()
	iq, err := os.ReadFile("../cmd/dmrrf/testdata/xpr8400_gain_sweep.cu8")
	if err != nil {
		t.Fatal(err)
	}
	for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
		at, after, ok := lockNCO(t, iq, 256000, tier)
		if !ok || math.Abs(at-xprCarrierHz) > iqLockOffsetBound || math.Abs(after-xprCarrierHz) > iqLockOffsetBound {
			t.Errorf("tier %d: oscillator at %.0f Hz at the lock and %.0f Hz %d0 ms later (locked %v), want within %d Hz of %d", tier-Tier0, at, after, iqAfterLock, ok, iqLockOffsetBound, xprCarrierHz)
		}
	}
}

const (
	iqLockOffsetBound = 150
	iqAfterLock       = 20
	xprCarrierHz      = 1400
)
