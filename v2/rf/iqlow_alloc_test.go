package rf

import "testing"

// TestAllocsLowIQ checks that a LowIQReceiver in caller memory allocates
// nothing in Init and WriteBaseband and, with the settings of the audio
// Tier 0 core, decodes every voice frame of a BS outbound call at 1400 Hz
// offset at 30 dB, at 9.6 and 12 kHz.
func TestAllocsLowIQ(t *testing.T) {
	slots, _ := benchStream(t, 3)
	x := noisyBaseband(slots, 1400, 30, 1)
	o := discOptions()
	o.MidTiming, o.Front96, o.BSSuspectRun, o.RawSearch = true, true, 3, 2
	for _, hz := range []int{9600, 12000} {
		y := lowIQ(x, hz)
		var r LowIQReceiver
		var got []Burst
		on := func(b *Burst) { got = append(got, *b) }
		if n := testing.AllocsPerRun(1, func() {
			if err := r.Init(LowIQConfig{Hz: hz, OnBurst: on, opt: &o}); err != nil {
				t.Fatal(err)
			}
		}); n != 0 {
			t.Errorf("%d Hz: Init: %v allocations", hz, n)
		}
		got = make([]Burst, 0, 4*len(slots)+16)
		const chunk = 1 << 9
		pos := 0
		if n := testing.AllocsPerRun(len(y)/chunk-2, func() {
			r.WriteBaseband(y[pos : pos+chunk])
			pos += chunk
		}); n != 0 {
			t.Errorf("%d Hz: %v allocations per WriteBaseband", hz, n)
		}
		r.WriteBaseband(y[pos:])
		r.Flush()
		if exact, total := exactFrames(slots, got); exact != total || total == 0 {
			t.Errorf("%d Hz: %d of %d voice frames exact", hz, exact, total)
		}
	}
}
