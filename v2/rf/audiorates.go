package rf

const (
	audioUpTaps   = 8
	audioUpPhases = 32
	audioUpCut    = 0.45
	audioUpBeta   = 8.0
	audioMinRate  = 8000
	audioMaxRate  = 1000000
)

func audioNative(rate int) int {
	for _, hz := range [...]int{2 * symbolRate, discRate / 2, discRate, 2 * discRate} {
		if rate <= hz {
			return hz
		}
	}
	return 2 * discRate
}
