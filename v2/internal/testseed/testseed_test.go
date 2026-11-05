package testseed

import (
	"math"
	"testing"
)

func TestUpperBound(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		fails, n int
		want     float64
	}{{0, 3, 0.7076}, {0, 100, 0.03622}, {5, 100, 0.1128}, {1, 1000, 0.005558}, {10, 10, 1}} {
		if got := UpperBound(c.fails, c.n); math.Abs(got-c.want) > 2e-4*max(1, c.want*10) {
			t.Errorf("UpperBound(%d, %d) = %.5g, want %.5g", c.fails, c.n, got, c.want)
		}
	}
}

func TestTrialStreams(t *testing.T) {
	t.Parallel()
	a, b, c := Of(42).Trial(3).Uint64(), Of(42).Trial(3).Uint64(), Of(42).Trial(4).Uint64()
	if a != b || a == c {
		t.Fatal("trial streams do not follow the trial index")
	}
	if New(t).Seed() != Mix(RunSeed(), NameHash(t.Name())) {
		t.Fatal("the Gen of a test does not come from the run seed and its name")
	}
}
