package rf

import "testing"

// iqDiscReceive runs the baseband x through iqRx at hz with OnBurst.
func iqDiscReceive(x []complex64, hz int) []Burst {
	o := discOptions()
	o.IQDisc, o.IQHz = true, hz
	var out []Burst
	r := mustNew(Config{Rate: basebandRate, Tier: Tier0, opt: &o, OnBurst: func(b *Burst) { out = append(out, *b) }})
	for k := 0; k < len(x); k += 4096 {
		r.WriteBaseband(x[k:min(k+4096, len(x))])
	}
	r.Flush()
	return out
}

// TestIQDiscLoopback receives a strong BS stream with a 1400 Hz carrier
// offset through iqRx at both discriminator rates and checks that every
// voice frame and data block after the first slot decodes, and that the
// oscillator found the offset.
func TestIQDiscLoopback(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 6)
	x := noisyBaseband(slots, 1400, 30, 3)
	for _, hz := range []int{discRate, discRate / 2} {
		sc := scoreBench(truth, iqDiscReceive(x, hz))
		for _, k := range []string{"ambe", "rate12", "rate34", "csbk", "emb", "slottype"} {
			// The receiver locks to the SYNC of the first slot, which it
			// does not decode.
			if s := sc[k]; s.Truth-s.Exact > 3 {
				t.Errorf("%d Hz: %s %d of %d exact", hz, k, s.Exact, s.Truth)
			}
		}
	}
	o := discOptions()
	o.IQDisc = true
	r := mustNew(Config{Rate: basebandRate, Tier: Tier0, opt: &o, OnBurst: func(*Burst) {}})
	r.WriteBaseband(x)
	if got := float64(r.iqd.conv.hz); got < 1300 || got > 1500 {
		t.Errorf("oscillator at %.0f Hz, want about 1400", got)
	}
}
