package rf

import (
	"math"
	"testing"
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
