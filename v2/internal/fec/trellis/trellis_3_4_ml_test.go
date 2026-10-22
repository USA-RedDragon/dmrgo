package trellis

import (
	"testing"
)

// scalarML is a plain Viterbi decoder with the metric of Decode. It returns
// the block whose path ends in state 0 with the least metric, that metric,
// and the number of such paths.
func scalarML(rx *[25]byte) (best [18]byte, metric, paths int) {
	const inf = 1 << 30
	var dib [98]byte
	for i, j := range interleave_matrix {
		dib[j] = dibitAt(rx, i)
	}
	stepCost := [4]int{0, 7, 10, 9}
	symCost := func(a, b byte) int {
		d := level(a) - level(b)
		if d < 0 {
			d = -d
		}
		return stepCost[d/2]
	}
	m := [8]int{0, inf, inf, inf, inf, inf, inf, inf}
	cnt := [8]int{1}
	var prev [49][8]byte
	for k := range 49 {
		next := [8]int{inf, inf, inf, inf, inf, inf, inf, inf}
		var nc [8]int
		for s := range 8 {
			if m[s] == inf {
				continue
			}
			for in := range 8 {
				pair := trellis34BranchDibits[s*8+in]
				c := m[s] + symCost(dib[2*k], pair>>2) + symCost(dib[2*k+1], pair&3)
				switch {
				case c < next[in]:
					next[in], nc[in], prev[k][in] = c, cnt[s], byte(s)
				case c == next[in]:
					nc[in] += cnt[s]
				}
			}
		}
		m, cnt = next, nc
	}
	st := byte(0)
	for k := 48; k >= 0; k-- {
		if k < 48 {
			bit := 3 * k
			v := uint16(st) << (13 - bit%8)
			best[bit/8] |= byte(v >> 8)
			if bit/8+1 < len(best) {
				best[bit/8+1] |= byte(v)
			}
		}
		st = prev[k][st]
	}
	return best, m[0], cnt[0]
}

func TestTrellis34_TwoBitErrorsMatchML(t *testing.T) {
	t.Parallel()

	data := [18]byte{0xb5, 0x81, 0x1a, 0x2c, 0x2b, 0x9c, 0xe1, 0xb5, 0xe3, 0x14, 0x86, 0xb2, 0xc0, 0xeb, 0x04, 0x37, 0x33, 0x6d}
	enc := Encode(data)
	var ok, tied, closer int
	for a := range 196 {
		for b := a + 1; b < 196; b++ {
			rx := enc
			rx[a/8] ^= 0x80 >> (a % 8)
			rx[b/8] ^= 0x80 >> (b % 8)
			got, res := Decode(rx)
			ml, metric, paths := scalarML(&rx)
			dec := Encode(got)
			if cost(&rx, &dec) != metric || res.Uncorrectable != (paths > 1) || paths == 1 && got != ml {
				t.Fatalf("bits %d and %d: %x %+v, plain Viterbi %x metric %d over %d paths", a, b, got, res, ml, metric, paths)
			}
			switch {
			case got == data && !res.Uncorrectable:
				ok++
			case paths > 1:
				tied++
			case metric < cost(&rx, &enc):
				closer++
			default:
				t.Fatalf("bits %d and %d: wrong block %x without a tie or a closer block", a, b, got)
			}
		}
	}
	t.Logf("of 19110 two-bit errors: %d corrected, %d tied, %d closer to another block", ok, tied, closer)
}
