//go:build tinygo

package rf

type audioRate struct{}

func (audioRate) resampling() bool { return false }

func (audioRate) resampleTaps() int { return 0 }

func audioRateOK(rate int, _ Tier) bool { return audioNative(rate) == rate }

func (r *AudioReceiver) setResample(int, int) {}

func (r *AudioReceiver) designResample(Tier) {}

func (r *AudioReceiver) resampled([]int16) {}
