package rf

import (
	"math"
	"testing"
)

// TestLogAdd checks logAdd against the exact log-sum over a span of
// differences.
func TestLogAdd(t *testing.T) {
	t.Parallel()
	var worst float64
	for i := range 20000 {
		d := float32(i) / 1000
		got := float64(logAdd(0, -d))
		want := math.Log1p(math.Exp(-float64(d)))
		worst = max(worst, math.Abs(got-want))
	}
	if worst > 5e-4 {
		t.Fatalf("logAdd off by %g nats", worst)
	}
	if v := logAdd(negInf, negInf); v > negInf/2 {
		t.Fatalf("logAdd(negInf, negInf) = %g", v)
	}
}
