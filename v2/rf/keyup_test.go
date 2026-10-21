package rf

import (
	"math"
	"testing"
)

func TestMSKeyUpStrobes(t *testing.T) {
	t.Parallel()
	fm, env, _ := directCalls(t, 6)
	for seed := range uint64(4) {
		a := discAudio(msBaseband(fm, env, 0, 60, 70+seed), benchAudioHz)
		var r *DiscReceiver
		stream, n := -1, 0
		var end float64
		on := func(b *Burst) {
			if b.Stream != stream {
				stream, n = b.Stream, 0
				if !r.tm.hold {
					t.Errorf("seed %d stream %d: the timing loop runs through the slot after the first burst", seed, b.Stream)
				}
			}
			base := r.first + int64(r.next-1)*slotSymbols + cachSymbols
			switch n++; n {
			case 1:
				end = strobeLag(r, base+79, base+131)
			case 2:
				if d := strobeLag(r, base+1, base+53) - end; math.Abs(d) > 0.035 {
					t.Errorf("seed %d stream %d: the timing moved by %.3f of a symbol between the first two bursts", seed, b.Stream, d)
				}
			}
		}
		var err error
		r, err = NewDisc(&DiscConfig{Hz: benchAudioHz, Tier: Tier0, OnBurst: on})
		if err != nil {
			t.Fatal(err)
		}
		for k := 0; k < len(a); k += 1024 {
			must(r.Write(a[k:min(k+1024, len(a))]))
		}
		r.Flush()
	}
}

func strobeLag(r *DiscReceiver, k0, k1 int64) float64 {
	lv := func(k int64) (float64, float64) {
		v := float64((r.sym(k) - r.dcHz) / (r.devScale * deviationHz))
		return v, max(-3, min(3, 2*math.Floor(v/2)+1))
	}
	var a [3][3]float64
	var b [3]float64
	for k := k0; k <= k1; k++ {
		v, s := lv(k)
		_, sn := lv(k + 1)
		_, sp := lv(k - 1)
		x := [3]float64{1, sn - sp, s}
		for i := range 3 {
			for j := range 3 {
				a[i][j] += x[i] * x[j]
			}
			b[i] += x[i] * (v - s)
		}
	}
	det := func(m [3][3]float64) float64 {
		return m[0][0]*(m[1][1]*m[2][2]-m[1][2]*m[2][1]) - m[0][1]*(m[1][0]*m[2][2]-m[1][2]*m[2][0]) + m[0][2]*(m[1][0]*m[2][1]-m[1][1]*m[2][0])
	}
	m := a
	for i := range 3 {
		m[i][1] = b[i]
	}
	return det(m) / det(a)
}
