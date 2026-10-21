package fsktx

import (
	"math/rand/v2"
	"slices"
	"testing"
)

func TestDroopRestarts(t *testing.T) {
	t.Parallel()
	cfg := AudioConfig{Tier: Tier1, Droop: DroopConfig{Hz: 10, LowHz: 1, Gain: 0.02}}
	a, err := NewAudio(cfg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewAudio(cfg)
	if err != nil {
		t.Fatal(err)
	}
	r := rand.New(rand.NewPCG(1, 2))
	levels := []int8{-3, -1, 1, 3}
	for range 20000 {
		a.AppendSymbol(nil, levels[r.IntN(4)])
	}
	var x, y []int16
	for i := range 2000 {
		s := int8(0)
		if i >= 64 {
			s = levels[r.IntN(4)]
		}
		x = a.AppendSymbol(x, s)
		y = b.AppendSymbol(y, s)
	}
	if !slices.Equal(x[640:], y[640:]) {
		t.Fatal("output depends on the symbols before a run of symbol 0")
	}
	if _, err := NewAudio(AudioConfig{Tier: Tier1, Droop: DroopConfig{Hz: 1, LowHz: 2, Gain: 0.1}}); err == nil {
		t.Fatal("LowHz above Hz was taken")
	}
	if _, err := NewAudio(AudioConfig{Tier: Tier1, Droop: DroopConfig{Hz: 10, Gain: 0.1, Floor: -1}}); err == nil {
		t.Fatal("a negative Floor was taken")
	}
	r48 := AudioConfig{Tier: Tier1, Hz: 96000, Rates: []RateDesign{{Hz: 48000, Droop: DroopConfig{Hz: 10, Gain: 0.4, Floor: 300}}}}
	c, err := NewAudio(r48)
	if err != nil {
		t.Fatal(err)
	}
	if c.dr.g != 0 {
		t.Fatal("the Droop of the 48000 Hz design reached 96000 Hz")
	}
	r48.Hz = 48000
	if c, err = NewAudio(r48); err != nil || c.dr.floor != 300 {
		t.Fatal("the Droop of the 48000 Hz design did not reach 48000 Hz", err)
	}
}
