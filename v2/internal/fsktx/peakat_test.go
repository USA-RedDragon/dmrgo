package fsktx

import (
	"errors"
	"testing"
)

func TestPeakAt(t *testing.T) {
	t.Parallel()
	low := AudioConfig{Hz: 48000, Tier: Tier1, Shape: 2, NoiseZerosHz: [4]float64{7000, 21000}, ShapeMinLevel: 500,
		ShapeLow: 1, NoiseLowZerosHz: [4]float64{9000}, NoiseLowPoleR: 0.9, CarrierHz: 3, CarrierMinLevel: 800}
	cfgs := []AudioConfig{
		{Hz: 48000, Tier: Tier1}, {Hz: 24000, Tier: Tier2}, {Hz: 22050, Tier: Tier1}, {Hz: 96000, Tier: Tier2}, low,
	}
	for _, c := range cfgs {
		base := c
		base.Level = 1
		a, err := NewAudio(base)
		if err != nil {
			t.Fatalf("%+v: %v", base, err)
		}
		for _, level := range []int{-1, 0, 1, 2, 499, 500, 501, 799, 800, 801, 3000, 15999, 16000, 16001} {
			c.Level = level
			got, gerr := a.PeakAt(level)
			b, werr := NewAudio(c)
			var want float64
			switch {
			case level == 0:
				werr = errLevel
			case werr == nil:
				want = b.Peak()
			}
			if got != want || !errors.Is(gerr, werr) {
				t.Errorf("%d Hz tier %d level %d: PeakAt %v, %v, want %v, %v", c.Hz, c.Tier, level, got, gerr, want, werr)
			}
		}
	}
	if _, err := new(Audio).PeakAt(1); err == nil {
		t.Error("PeakAt of a Tier0 Audio")
	}
}
