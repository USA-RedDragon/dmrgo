package rf

import (
	"fmt"
	"testing"
)

// iq0Receive runs the baseband x through the lean front of opt in writes
// of chunk samples, and returns the bursts and the oscillator at the end.
func iq0Receive(x []complex64, o options, chunk int) ([]Burst, float32) {
	var out []Burst
	r := mustNew(Config{Rate: basebandRate, Tier: Tier0, opt: &o, OnBurst: func(b *Burst) { out = append(out, *b) }})
	for k := 0; k < len(x); k += chunk {
		r.WriteBaseband(x[k:min(k+chunk, len(x))])
	}
	hz := r.iqd.conv.hz
	r.Flush()
	return out, hz
}

// iq0Options returns the settings of the lean front with front, at hz.
func iq0Options(front, hz int) options {
	o := discOptions()
	o.IQDisc, o.IQHz, o.IQFront, o.IQPoly, o.IQBuf = true, hz, front, 3, 64
	return o
}

// TestIQ0Loopback receives a strong BS stream with a 1400 Hz carrier
// offset through each lean front at both rates, and checks that every
// voice frame and data block after the first slot decodes, and that the
// oscillator found the offset.
func TestIQ0Loopback(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 6)
	x := noisyBaseband(slots, 1400, 30, 3)
	for _, front := range []int{1, 2, 3} {
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

// TestIQ0Writes checks that the bursts of the lean front do not depend on
// how the input splits into writes, on a weak stream with a carrier
// offset, whose bursts carry errors.
func TestIQ0Writes(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 6)
	x := noisyBaseband(slots, 1400, 6, 4)
	for _, front := range []int{1, 2, 3} {
		o := iq0Options(front, 2*symbolRate)
		want, _ := iq0Receive(x, o, 4096)
		got, _ := iq0Receive(x, o, 7)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("front %d: %d bursts in writes of 7 samples differ from %d in writes of 4096", front, len(got), len(want))
		}
	}
}

// TestAllocsIQ0 checks that the lean front allocates nothing per
// WriteBaseband.
func TestAllocsIQ0(t *testing.T) {
	slots, _ := benchStream(t, 6)
	x := noisyBaseband(slots, 1400, 10, 5)
	o := iq0Options(3, 2*symbolRate)
	r := mustNew(Config{Rate: basebandRate, Tier: Tier0, opt: &o, OnBurst: func(*Burst) {}})
	const chunk = 4096
	r.WriteBaseband(x[:chunk])
	i := chunk
	if n := testing.AllocsPerRun(20, func() {
		r.WriteBaseband(x[i : i+chunk])
		i += chunk
	}); n != 0 {
		t.Errorf("%v allocations per WriteBaseband", n)
	}
}
