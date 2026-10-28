package fsktx

import "testing"

func TestMCUAudioPEBounds(t *testing.T) {
	t.Parallel()
	worst := make([]int8, 0, 4000)
	for i := range 4000 {
		s := int8(3)
		if i/7%2 == 1 {
			s = -3
		}
		worst = append(worst, s)
	}
	runs := [][]int8{goldenSymbols(4000), worst}
	for _, level := range []int{1, 1040, 15000, 16000} {
		for _, dir := range []int{1, -1} {
			off := edgeOffsetPE(t, level, dir)
			lo, hi := peBounds(int32((level<<15 + tableUnits/2) / tableUnits))
			for _, syms := range runs {
				var m MCUAudioPE
				if err := m.Init(level, off); err != nil {
					t.Fatal(err)
				}
				var buf [peSamples]int16
				for i, s := range append(syms, make([]int8, 4*peSpan)...) {
					if n := m.Symbol(buf[:], s); n != peSamples {
						t.Fatalf("symbol %d: %d samples", i, n)
					}
					for _, v := range buf {
						if d := int(v) - off; d < lo || d > hi {
							t.Fatalf("level %d offset %d, symbol %d: %d from the offset, out of %d to %d", level, off, i, d, lo, hi)
						}
					}
				}
				for _, v := range buf {
					if d := int(v) - off; d < -1-level/1000 || d > 1+level/1000 {
						t.Errorf("level %d offset %d: the tail is %d from the offset after %d symbols 0", level, off, d, 4*peSpan)
					}
				}
			}
		}
	}
	var m MCUAudioPE
	var buf [peSamples]int16
	if m.Symbol(buf[:], 3) != 0 || m.Init(0, 0) == nil || m.Init(16001, 0) == nil {
		t.Error("MCUAudioPE took a symbol before Init or a level out of range")
	}
	if err := m.Init(15000, 0); err != nil || m.Symbol(buf[:9], 3) != 0 {
		t.Error("MCUAudioPE wrote to a short dst")
	}
}

func edgeOffsetPE(t *testing.T, level, dir int) int {
	t.Helper()
	var m MCUAudioPE
	ok, bad := 0, dir*(1<<15)
	for bad-ok > 1 || ok-bad > 1 {
		mid := (ok + bad) / 2
		if m.Init(level, mid) == nil {
			ok = mid
		} else {
			bad = mid
		}
	}
	return ok
}
