package rf

import (
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func TestCombReleaseOnce(t *testing.T) {
	var got []float64
	c := &combRx{last: math.Inf(-1), mode: 2, on: func(b *Burst) { got = append(got, b.Seconds) }}
	for i := range c.pend {
		c.pend[i] = make([]combBurst, 0, 16)
	}
	at := func(i int, s float64) { c.pend[i] = append(c.pend[i], combBurst{b: Burst{Seconds: s}}) }
	at(0, 0.09)
	at(0, 0.03)
	at(1, 0.0301)
	at(1, 0.06)
	at(1, 0.1201)
	c.release(0.0302)
	at(0, 0.0302)
	at(0, 0.12)
	c.release(math.Inf(1))
	want := []float64{0.03, 0.06, 0.09, 0.12}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestCombModeClean(t *testing.T) {
	c := &combRx{mode: 12}
	var x, y combBurst
	x.b.Voice, x.b.HasFixed, x.s.hasFlips = enums.VoiceBurstA, true, true
	y.b = x.b
	f := y.b.Data.VoiceSlots()
	f[0][5] ^= 0x10
	y.b.Data.SetVoiceSlots(f)
	y.b.Fixed = y.b.Data
	took := func() bool {
		return c.pick(&x, &y).Fixed.VoiceSlots()[0] == f[0]
	}
	n := 0
	for !took() {
		n++
		if n > 100 {
			t.Fatal("never takes the clean frame of Tier2")
		}
	}
	if n < 10 {
		t.Fatalf("takes the clean frame of Tier2 after %d frames, before its share grew", n)
	}
	g := f
	g[0][6] ^= 0x01
	y.b.Data.SetVoiceSlots(g)
	if took() {
		t.Fatal("takes a frame of Tier2 that its decode changed")
	}
}
