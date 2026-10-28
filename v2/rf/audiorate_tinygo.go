//go:build tinygo

package rf

type audioRate struct{}

func (audioRate) resampling() bool { return false }

func (audioRate) resampleTaps() int { return 0 }

func audioRateOK(rate int, _ Tier) bool { return rate >= audioMinRate && rate <= 2*discRate }

func (r *AudioReceiver) designResample(Tier) {}

func (r *AudioReceiver) resampled([]int16) {}
