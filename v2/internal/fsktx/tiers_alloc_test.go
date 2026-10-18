//go:build !tinygo

package fsktx

import "testing"

// TestAllocs checks that the modulators allocate nothing per symbol once
// dst has room.
func TestAllocs(t *testing.T) {
	syms := []int8{3, -1, 1, -3, 0, 3, 3, -3}
	for _, cfg := range audioTiers() {
		a, err := NewAudio(cfg)
		if err != nil {
			t.Fatal(err)
		}
		dst := make([]int16, 0, 64)
		if n := testing.AllocsPerRun(100, func() {
			for _, s := range syms {
				dst = a.AppendSymbol(dst[:0], s)
			}
		}); n != 0 {
			t.Errorf("%+v: %v allocations per run", cfg, n)
		}
	}
	for _, cfg := range iqTiers() {
		q, err := NewIQ(cfg)
		if err != nil {
			t.Fatal(err)
		}
		q.Key(true)
		d8 := make([]byte, 0, 4096)
		d16 := make([]int16, 0, 4096)
		if n := testing.AllocsPerRun(100, func() {
			for _, s := range syms {
				d8 = q.AppendS8(d8[:0], s)
				d16 = q.AppendS16(d16[:0], s)
			}
		}); n != 0 {
			t.Errorf("%+v: %v allocations per run", cfg, n)
		}
	}
	var m MCUAudio
	var mq MCUIQ
	if err := m.Init(Table48k(), 15000, 0); err != nil {
		t.Fatal(err)
	}
	if err := mq.Init(48000, 1000, 48); err != nil {
		t.Fatal(err)
	}
	var buf [64]int16
	if n := testing.AllocsPerRun(100, func() {
		for _, s := range syms {
			m.Symbol(buf[:], s)
			mq.Symbol(buf[:], s)
		}
	}); n != 0 {
		t.Errorf("MCU tiers: %v allocations per run", n)
	}
}
