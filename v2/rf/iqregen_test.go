//go:build !tinygo

package rf

import (
	"math"
	"testing"
)

func TestIQRegenAudioAligned(t *testing.T) {
	t.Parallel()
	x := make([]complex64, basebandRate)
	for i := range x {
		p := 2*math.Pi*1500*float64(i)/basebandRate - 900/700.0*math.Cos(2*math.Pi*700*float64(i)/basebandRate)
		x[i] = complex(float32(math.Cos(p)), float32(math.Sin(p)))
	}
	r := mustNew(RxConfig{Rate: basebandRate, Tier: Tier1, Format: IQFormatCF32})
	q := r.host().iqd
	for _, n := range []int{4000, 2400 + 333, 2400 + 1001} {
		q.writeBaseband(x[:n])
		for _, v := range x[n : n+700] {
			q.sample(v)
		}
		x = x[n+700:]
		if q.d.locked {
			t.Fatal("locked on a tone")
		}
		live := append([]int16(nil), q.audio...)
		got := append([]int16(nil), live...)
		if len(live) == 0 || !q.regenAudio(got) {
			t.Fatalf("no audio to regenerate (%d pending)", len(live))
		}
		worst := 0
		for i := range live {
			worst = max(worst, abs(int(got[i])-int(live[i])))
		}
		if worst > 2 {
			t.Errorf("%d pending samples: the regenerated audio differs from the converter by up to %d units", len(live), worst)
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
