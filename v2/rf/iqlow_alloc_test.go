package rf

import "testing"

func TestAllocsLowIQ(t *testing.T) {
	slots, _ := benchStream(t, 3)
	x := noisyBaseband(slots, 1400, 30, 1)
	o := discOptions()
	o.MidTiming, o.Front96, o.BSSuspectRun, o.RawSearch = true, true, 3, 2
	for _, hz := range []int{9600, 12000} {
		y := lowIQ(x, hz)
		var r Receiver
		var got []Burst
		on := func(bs []Burst) { got = append(got, bs...) }
		if n := testing.AllocsPerRun(1, func() {
			if err := r.Init(RxConfig{Rate: hz, Format: IQFormatCF32, OnBursts: on}); err != nil {
				t.Fatal(err)
			}
		}); n != 0 {
			t.Errorf("%d Hz: Init: %v allocations", hz, n)
		}
		r.setLow(RxConfig{Rate: hz, Format: IQFormatCF32, OnBursts: on}, &o)
		got = make([]Burst, 0, 4*len(slots)+16)
		in := cf32(y)
		const chunk = 1 << 12
		pos := 0
		if n := testing.AllocsPerRun(len(in)/chunk-2, func() {
			_, _ = r.Write(in[pos : pos+chunk])
			pos += chunk
		}); n != 0 {
			t.Errorf("%d Hz: %v allocations per Write", hz, n)
		}
		_, _ = r.Write(in[pos:])
		_ = r.Flush()
		if exact, total := exactFrames(slots, got); exact != total || total == 0 {
			t.Errorf("%d Hz: %d of %d voice frames exact", hz, exact, total)
		}
	}
}
