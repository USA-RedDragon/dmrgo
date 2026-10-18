package fsktx

import "testing"

// goldenSymbols returns n pseudo-random symbols with a run of 0 at the
// start and the end.
func goldenSymbols(n int) []int8 {
	out := make([]int8, n)
	x := uint32(12345)
	for i := 8; i < n-8; i++ {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		out[i] = int8(2*(x>>30)) - 3
	}
	return out
}

// fnv folds v into the FNV-1a hash h.
func fnv(h uint32, v int16) uint32 {
	for _, b := range [2]byte{byte(v), byte(uint16(v) >> 8)} {
		h ^= uint32(b)
		h *= 16777619
	}
	return h
}

// TestGolden checks the output of the Tier0 modulators against digests,
// so that a change of their samples, or a difference between compilers,
// shows.
func TestGolden(t *testing.T) {
	t.Parallel()
	syms := goldenSymbols(2000)
	var buf [160]int16
	cases := []struct {
		name string
		run  func() uint32
		want uint32
	}{
		{"audio 24k", func() uint32 {
			var m MCUAudio
			if err := m.Init(Table24k(), 15000, 0); err != nil {
				t.Fatal(err)
			}
			h := uint32(2166136261)
			for _, s := range syms {
				for _, v := range buf[:m.Symbol(buf[:], s)] {
					h = fnv(h, v)
				}
			}
			return h
		}, 0xf21adadc},
		{"audio 48k dac", func() uint32 {
			var m MCUAudio
			if err := m.Init(Table48k(), 1100, 2048); err != nil {
				t.Fatal(err)
			}
			h := uint32(2166136261)
			for _, s := range syms {
				for _, v := range buf[:m.Symbol(buf[:], s)] {
					h = fnv(h, v)
				}
			}
			return h
		}, 0x9681552d},
		{"iq 24k at 96k", func() uint32 {
			var m MCUIQ
			if err := m.Init(96000, -2000, 96); err != nil {
				t.Fatal(err)
			}
			h := uint32(2166136261)
			for i, s := range syms {
				m.Key(i > 4 && i < len(syms)-20)
				for _, v := range buf[:m.Symbol(buf[:], s)] {
					h = fnv(h, v)
				}
			}
			return h
		}, 0xad77a3b6},
	}
	for _, c := range cases {
		if got := c.run(); got != c.want {
			t.Errorf("%s: digest %#x, want %#x", c.name, got, c.want)
		}
	}
}
