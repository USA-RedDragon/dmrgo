package rf

import (
	"math"
	"testing"
)

func halve48(x []int16) []int16 {
	out := make([]int16, 0, len(x)/2)
	at := func(i int) int32 {
		if i < 0 {
			return 0
		}
		return int32(x[i])
	}
	for n := 1; n < len(x); n += 2 {
		c := n - halfTaps/2
		acc := int32(discHalfQ15[0]) * at(c)
		for k, t := range discHalfQ15[1:] {
			acc += int32(t) * (at(c-2*k-1) + at(c+2*k+1))
		}
		out = append(out, int16(max(min((acc+1<<14)>>15, math.MaxInt16), math.MinInt16)))
	}
	return out
}

func TestDisc48MatchesHalved(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 6)
	x := noisyBaseband(slots, 1400, 30, 1)
	a48 := discAudio(x, 48000)
	a24 := halve48(a48)
	for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
		var got, want []Burst
		r48, err := NewAudioReceiver(&AudioRxConfig{Rate: 48000, Tier: tier, OnBursts: each(func(b *Burst) { got = append(got, *b) })})
		if err != nil {
			t.Fatal(err)
		}
		r24, err := NewAudioReceiver(&AudioRxConfig{Rate: 24000, Tier: tier, OnBursts: each(func(b *Burst) { want = append(want, *b) })})
		if err != nil {
			t.Fatal(err)
		}
		for k := 0; k < len(a48); k += 999 {
			must(r48.writeAll(a48[k:min(k+999, len(a48))]))
		}
		r48.flushAll()
		must(r24.writeAll(a24))
		r24.flushAll()
		if len(got) == 0 || len(got) != len(want) {
			t.Fatalf("tier %d: %d bursts at 48 kHz, %d at 24 kHz halved by hand", tier, len(got), len(want))
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("tier %d: burst %d differs", tier, i)
			}
		}
		sc := scoreBench(truth, got)
		if s := sc["ambe"]; s.Truth == 0 || s.Exact < s.Truth {
			t.Errorf("tier %d: %d of %d voice frames", tier, s.Exact, s.Truth)
		}
	}
}

func TestDisc48InitAppend(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 3)
	a := discAudio(noisyBaseband(slots, 1400, 20, 1), 48000)
	var r AudioReceiver
	if err := r.Init(AudioRxConfig{Rate: 48000}); err != nil {
		t.Fatal(err)
	}
	var got []Burst
	for k := range a {
		got = r.AppendBursts(got, a[k:k+1])
	}
	got = r.AppendFlush(got)
	var s AudioReceiver
	if err := s.Init(AudioRxConfig{Rate: 24000}); err != nil {
		t.Fatal(err)
	}
	want := s.AppendFlush(s.AppendBursts(nil, halve48(a)))
	if len(want) == 0 || len(got) != len(want) {
		t.Fatalf("%d bursts at 48 kHz, %d at 24 kHz", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("burst %d differs", i)
		}
	}
}
