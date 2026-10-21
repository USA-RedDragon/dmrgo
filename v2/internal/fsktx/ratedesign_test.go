package fsktx

import (
	"slices"
	"testing"
)

func TestRateDesignPicksByHz(t *testing.T) {
	run := func(c AudioConfig) []int16 {
		a, err := NewAudio(c)
		if err != nil {
			t.Fatal(err)
		}
		var out []int16
		for _, s := range newTestStream(t, 3, 40).syms {
			out = a.AppendSymbol(out, s)
		}
		return out
	}
	base := AudioConfig{Tier: Tier1, Exact: true, Pulse: PulseConfig{Flat: true}}
	rates := []RateDesign{
		{Hz: 24000, ExactPow: 0.9, Gain: 1.001, FlatGain: 0.002, CarrierHz: 0.5},
		{Hz: 48000, ExactPow: 0.2, Gain: 0.999, FlatGain: 0.0015, CarrierHz: 0.01},
	}
	for _, r := range rates {
		direct := base
		direct.Hz, direct.ExactPow, direct.Pulse.Gain, direct.Pulse.FlatGain, direct.CarrierHz = r.Hz, r.ExactPow, r.Gain, r.FlatGain, r.CarrierHz
		table := base
		table.Hz, table.Rates = r.Hz, rates
		plain := base
		plain.Hz = r.Hz
		got := run(table)
		if !slices.Equal(run(direct), got) || slices.Equal(run(plain), got) {
			t.Fatalf("%d Hz: the table entry is not the same fields", r.Hz)
		}
	}
	plain := base
	plain.Hz = 96000
	table := plain
	table.Rates = rates
	if !slices.Equal(run(plain), run(table)) {
		t.Fatal("a rate with no entry is not the plain config")
	}
}
