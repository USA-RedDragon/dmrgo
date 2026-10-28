package fsktx

import (
	"slices"
	"testing"
)

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

func fnv(h uint32, v int16) uint32 {
	for _, b := range [2]byte{byte(v), byte(uint16(v) >> 8)} {
		h ^= uint32(b)
		h *= 16777619
	}
	return h
}

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
		}, 0x010328d9},
	}
	for _, c := range cases {
		if got := c.run(); got != c.want {
			t.Errorf("%s: digest %#x, want %#x", c.name, got, c.want)
		}
	}
}

func TestGoldenLimits(t *testing.T) {
	t.Parallel()
	syms := goldenSymbols(3000)
	for i := range syms {
		switch i % 41 {
		case 7:
			syms[i] = int8(i)
		case 19:
			syms[i] = -128
		}
	}
	audio := func(tab AudioTable, level, offset int) func() uint32 {
		return func() uint32 {
			var m MCUAudio
			if err := m.Init(tab, level, offset); err != nil {
				t.Fatal(err)
			}
			var buf [maxSamples]int16
			h := uint32(2166136261)
			for i, s := range syms {
				if i%13 == 5 {
					m.Symbol(buf[:m.Samples()-1], s)
				}
				for _, v := range buf[:m.Symbol(buf[:], s)] {
					h = fnv(h, v)
				}
			}
			return h
		}
	}
	iq := func(rate, off, ramp int, key func(i int) bool) func() uint32 {
		return func() uint32 {
			var m MCUIQ
			if err := m.Init(rate, off, ramp); err != nil {
				t.Fatal(err)
			}
			var buf [2 * maxSamples << 3]int16
			h := uint32(2166136261)
			for i, s := range syms {
				m.Key(key(i))
				for _, v := range buf[:m.Symbol(buf[:], s)] {
					h = fnv(h, v)
				}
			}
			return h
		}
	}
	cases := []struct {
		name string
		run  func() uint32
		want uint32
	}{
		{"audio 24k 16000 high", audio(Table24k(), 16000, 2603), 0x6d32f347},
		{"audio 48k 16000 low", audio(Table48k(), 16000, -2482), 0xb07d0f48},
		{"audio 24k 1 high", audio(Table24k(), 1, 32765), 0xd48c999d},
		{"audio 48k 1 low", audio(Table48k(), 1, -32766), 0x2296d741},
		{"iq 48k high step", iq(48000, 11999, 1, func(i int) bool { return i%9 < 5 }), 0xbe1382f7},
		{"iq 192k low ramp", iq(192000, -47999, 1<<15, func(i int) bool { return i < 2000 }), 0x8f64b885},
		{"iq 96k every symbol", iq(96000, 23999, 7, func(i int) bool { return i&1 == 0 }), 0x20a76fe2},
	}
	for _, c := range cases {
		if got := c.run(); got != c.want {
			t.Errorf("%s: digest %#x, want %#x", c.name, got, c.want)
		}
	}
}

func TestMCUIQUnaligned(t *testing.T) {
	t.Parallel()
	syms := goldenSymbols(600)
	for _, rate := range []int{96000, 192000} {
		var a, b MCUIQ
		if err := a.Init(rate, -2000, 96); err != nil {
			t.Fatal(err)
		}
		b = a
		var x [2 * maxSamples << 3]int16
		var y [2*maxSamples<<3 + 1]int16
		for i, s := range syms {
			a.Key(i > 4 && i < len(syms)-20)
			b.Key(i > 4 && i < len(syms)-20)
			n, m := a.Symbol(x[:], s), b.Symbol(y[1:], s)
			if n != m || !slices.Equal(x[:n], y[1:1+n]) {
				t.Fatalf("%d Hz, symbol %d: the output at an odd address differs", rate, i)
			}
		}
	}
}
