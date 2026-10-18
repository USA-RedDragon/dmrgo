//go:build !tinygo

package fsktx

import "testing"

// TestAllocsMatrix checks that no modulator of the matrix allocates per
// symbol once dst has room, through keying and ramps and invalid symbols,
// and that Init of the MCU modulators allocates nothing.
func TestAllocsMatrix(t *testing.T) {
	syms := []int8{3, -1, 1, -3, 0, 3, 3, -3, 7, -128, 127, 1}
	units := append(nominal(txFull()), offsetUnits(false)...)
	for _, u := range units {
		if u.rate > 2400000 {
			continue
		}
		m := u.mustMod(t)
		dst := make([]int16, 0, 2*len(syms)*(u.rate/SymbolRate+2))
		k := false
		if n := testing.AllocsPerRun(20, func() {
			dst = dst[:0]
			for _, s := range syms {
				k = !k
				m.key(k || s > 0)
				dst = m.emit(dst, s)
			}
		}); n != 0 {
			t.Errorf("%v: %v allocations per run", u, n)
		}
	}
	var a MCUAudio
	var q MCUIQ
	if n := testing.AllocsPerRun(20, func() {
		_ = a.Init(Table24k(), 1080, 2048)
		_ = a.Init(Table48k(), 0, 0)
		_ = q.Init(192000, -47999, 1<<15)
		_ = q.Init(1, 0, 0)
	}); n != 0 {
		t.Errorf("Init: %v allocations per run", n)
	}
}
