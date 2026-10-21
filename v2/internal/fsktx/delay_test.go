//go:build !tinygo

package fsktx

import (
	"slices"
	"testing"
)

func TestIQDelay(t *testing.T) {
	syms := []int8{3, -1, 1, -3, 3, 3, -3, -1, 1, 1}
	for _, tier := range []Tier{Tier1, Tier2} {
		run := func(d int) [][]int16 {
			q, err := NewIQ(IQConfig{Rate: 96000, Tier: tier, Delay: d})
			if err != nil {
				t.Fatal(err)
			}
			q.Key(true)
			var out [][]int16
			for i := range 40 {
				var s int8
				if i < len(syms) {
					s = syms[i]
				}
				out = append(out, q.AppendS16(nil, s))
			}
			return out
		}
		a, b := run(0), run(8)
		for i := 14; i < 32; i++ {
			if !slices.Equal(a[i-8], b[i]) {
				t.Fatalf("tier %v: symbol %d of Delay 8 is not symbol %d of Delay 0", tier, i, i-8)
			}
		}
	}
	if _, err := NewIQ(IQConfig{Rate: 96000, Tier: Tier1, Delay: maxDelay + 1}); err == nil {
		t.Fatal("Delay over maxDelay passed")
	}
}
