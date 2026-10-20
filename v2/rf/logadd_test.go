package rf

import (
	"math"
	"testing"
)

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

func TestJacobianSlope(t *testing.T) {
	t.Parallel()
	for i := range 96 {
		if d := jacobian[i+1] - jacobian[i]; jacobianSlope[i] != d {
			t.Fatalf("jacobianSlope[%d] = %g, want %g", i, jacobianSlope[i], d)
		}
	}
	for i := 97; i < len(jacobian); i++ {
		if jacobian[i] != 0 || jacobianSlope[i] != 0 {
			t.Fatalf("padding at %d is not zero", i)
		}
	}
}
