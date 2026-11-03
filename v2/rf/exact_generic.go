//go:build !amd64 || tinygo

package rf

func xtISA() uint8 {
	return 0
}

func (t *xtrellis) forwardSIMD(_, _, _, _, _, _, _ []float32, _ *xlanes, _, top float32, _ bool) (float32, bool) {
	return top, false
}

func (t *xtrellis) backwardSIMD(_, _, _, _, _, _, _, _ []float32, _ *xlanes, _ float32, _ *[4]float32) bool {
	return false
}

func (t *xtrellis) driftSIMD(_ []float32) bool {
	return false
}

func (t *xtrellis) corrPrep() bool {
	return false
}

func (t *xtrellis) corrSIMD(_, _ []complex64, _ complex64, _ float32) float32 {
	return 0
}
