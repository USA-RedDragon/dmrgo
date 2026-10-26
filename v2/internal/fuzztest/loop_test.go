package fuzztest_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/fuzztest"
)

func TestLoop(t *testing.T) {
	t.Parallel()
	for n := range 6 {
		for from := range uint8(7) {
			for to := range uint8(7) {
				for _, reps := range []uint8{0, 1, 3} {
					for _, limit := range []int{4, 100} {
						var want, wantRep []int
						lo := min(int(from), n)
						hi := max(min(int(to), n), lo)
						for s := range hi {
							want, wantRep = append(want, s), append(wantRep, 0)
						}
						for r := range int(reps) {
							for s := lo; s < hi; s++ {
								want, wantRep = append(want, s), append(wantRep, r+1)
							}
						}
						for s := hi; s < n; s++ {
							want, wantRep = append(want, s), append(wantRep, 0)
						}
						want, wantRep = want[:min(len(want), limit)], wantRep[:min(len(wantRep), limit)]
						gotLo, gotHi, total := fuzztest.Loop(n, from, to, reps, limit)
						if gotLo != lo || gotHi != hi || total != len(want) {
							t.Fatalf("Loop(%d, %d, %d, %d, %d) = %d, %d, %d, want %d, %d, %d", n, from, to, reps, limit, gotLo, gotHi, total, lo, hi, len(want))
						}
						for s := range total {
							if i := fuzztest.Index(s, lo, hi, reps); i != want[s] {
								t.Fatalf("n %d from %d to %d reps %d: Index(%d) = %d, want %d", n, from, to, reps, s, i, want[s])
							}
							if r := fuzztest.Repeat(s, lo, hi, reps); r != wantRep[s] {
								t.Fatalf("n %d from %d to %d reps %d: Repeat(%d) = %d, want %d", n, from, to, reps, s, r, wantRep[s])
							}
						}
					}
				}
			}
		}
	}
}
