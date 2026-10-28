package rf

import (
	"fmt"
	"testing"
)

func iq0Receive(x []complex64, o options, chunk int) ([]Burst, float32) {
	var out []Burst
	r := mustNewOpt(RxConfig{Rate: basebandRate, Tier: Tier0, OnBursts: each(func(b *Burst) { out = append(out, *b) }), Format: IQFormatCU8}, o)
	for k := 0; k < len(x); k += chunk {
		must(r.writeBB(x[k:min(k+chunk, len(x))]))
	}
	var hz float32
	if r.host().iq0 != nil {
		hz = r.host().iq0.hz
	} else {
		hz = r.host().iqd.conv.hz
	}
	r.flushAll()
	return out, hz
}

func TestIQ0Loopback(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 6)
	x := noisyBaseband(slots, 1400, 30, 3)
	for _, front := range []int{1, 2, 3, 4, 5} {
		for _, hz := range []int{discRate / 2, 2 * symbolRate} {
			out, nco := iq0Receive(x, iq0Options(front, hz), 4096)
			sc := scoreBench(truth, out)
			for _, k := range []string{"ambe", "rate12", "rate34", "csbk", "emb", "slottype"} {
				if s := sc[k]; s.Truth-s.Exact > 3 {
					t.Errorf("front %d at %d Hz: %s %d of %d exact", front, hz, k, s.Exact, s.Truth)
				}
			}
			if nco < 1300 || nco > 1500 {
				t.Errorf("front %d at %d Hz: oscillator at %.0f Hz, want about 1400", front, hz, nco)
			}
		}
	}
}

func TestIQ0Writes(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 6)
	x := noisyBaseband(slots, 1400, 6, 4)
	for _, front := range []int{1, 2, 3, 4, 5} {
		o := iq0Options(front, 2*symbolRate)
		want, _ := iq0Receive(x, o, 4096)
		got, _ := iq0Receive(x, o, 7)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("front %d: %d bursts in writes of 7 samples differ from %d in writes of 4096", front, len(got), len(want))
		}
	}
}
