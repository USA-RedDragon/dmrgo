//go:build tinygo

package rf

type leanPow struct{}

func newIQPower(int, float64) *iqPower {
	return nil
}

func (r *leanCore) setPower(*iqPower) {}

func (r *leanCore) slotPower(int64) (on, off float32, ok bool) {
	return 0, 0, false
}

type iqPower struct{}

func (*iqPower) push(complex64) {}
