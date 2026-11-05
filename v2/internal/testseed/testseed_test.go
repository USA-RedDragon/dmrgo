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

func TestRateBars(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		got  bool
		want bool
	}{
		{"0.8 of 400 below 0.9", RateBelow(320, 400, 0.9), true},
		{"0.88 of 400 below 0.9", RateBelow(352, 400, 0.9), false},
		{"all below 0.99", RateBelow(10, 10, 0.99), false},
		{"0 of 10 below 0.5", RateBelow(0, 10, 0.5), false},
		{"0 of 20 below 0.5", RateBelow(0, 20, 0.5), true},
		{"5 of 1000 above 0.001", RateAbove(5, 1000, 0.001), false},
		{"12 of 1000 above 0.001", RateAbove(12, 1000, 0.001), true},
		{"0 above 0", RateAbove(0, 10, 0), false},
		{"1 above 0", RateAbove(1, 10, 0), true},
		{"14 of 1000 against 0 of 1000", RateHigher(14, 1000, 0, 1000), true},
		{"13 of 1000 against 0 of 1000", RateHigher(13, 1000, 0, 1000), false},
		{"30 of 1000 against 20 of 1000", RateHigher(30, 1000, 20, 1000), false},
	} {
		if c.got != c.want {
			t.Errorf("%s: %v, want %v", c.name, c.got, c.want)
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
