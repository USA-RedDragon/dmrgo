package rf

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

// TestSymbolBitErrors checks the dibit distance of every pair of symbols
// against the dibit map of ETSI TS 102 361-1 Table 10.3.
func TestSymbolBitErrors(t *testing.T) {
	t.Parallel()
	for _, a := range []int8{-3, -1, 1, 3} {
		for _, b := range []int8{-3, -1, 1, 3} {
			var want int
			for _, d := range [][2]unpacked.Bit{{0, 1}, {0, 0}, {1, 0}, {1, 1}} {
				for _, e := range [][2]unpacked.Bit{{0, 1}, {0, 0}, {1, 0}, {1, 1}} {
					if symbolOf(d[0], d[1]) == a && symbolOf(e[0], e[1]) == b {
						want = int(d[0]^e[0]) + int(d[1]^e[1])
					}
				}
			}
			if got := symbolBitErrors(a, b); got != want {
				t.Errorf("symbols %d and %d: %d bit errors, want %d", a, b, got, want)
			}
		}
	}
}
