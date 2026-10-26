package ambe

import "testing"

func TestScrambleValues(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		a    uint16
		want uint32
	}{
		{0, 0x42CC47},
		{1, 0x19D6FE},
		{2, 0x304729},
		{3, 0x6B2CD0},
		{4, 0x60BF47},
		{5, 0x39650E},
		{10, 0xD7B745},
		{100, 0x16C05E},
	} {
		if got := scramble(tc.a); got != tc.want>>1 {
			t.Errorf("scramble(%d) = %#06x, want %#06x", tc.a, got, tc.want>>1)
		}
	}
}

func TestScrambleBitWidth(t *testing.T) {
	t.Parallel()

	for a := range uint16(4096) {
		if v := scramble(a); v>>23 != 0 {
			t.Errorf("scramble(%d) = %#08x is wider than 23 bits", a, v)
		}
	}
}

func TestScrambleDistribution(t *testing.T) {
	t.Parallel()

	var counts [23]int
	for a := range uint16(4096) {
		v := scramble(a)
		for bit := range counts {
			counts[bit] += int(v >> bit & 1)
		}
	}
	for bit, n := range counts {
		if n < 2048-409 || n > 2048+409 {
			t.Errorf("bit %d set %d times out of 4096, want 2048±409", bit, n)
		}
	}
}

func BenchmarkScramble(b *testing.B) {
	a := uint16(0)
	for b.Loop() {
		a++
		_ = scramble(a & 0xFFF)
	}
}
