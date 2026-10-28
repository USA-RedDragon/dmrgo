package fsktx

import (
	"math"
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
		}, 0xa4a76fa1},
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
		}, 0x8486b516},
		{"iq 96k", func() uint32 {
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
		}, 0x34311e16},
	}
	for _, c := range cases {
		if got := c.run(); got != c.want {
			t.Errorf("%s: digest %#x, want %#x", c.name, got, c.want)
		}
	}
}

func edgeOffset(t *testing.T, tab AudioTable, level, dir int) int {
	t.Helper()
	var m MCUAudio
	ok, bad := 0, dir*(math.MaxInt16+1)
	for bad-ok > 1 || ok-bad > 1 {
		mid := (ok + bad) / 2
		if m.Init(tab, level, mid) == nil {
			ok = mid
		} else {
			bad = mid
		}
	}
	if m.Init(tab, level, ok) != nil || m.Init(tab, level, ok+dir) == nil {
		t.Fatalf("level %d: no edge offset at %d", level, ok)
	}
	return ok
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
			prev := int32(offset)
			for i, s := range syms {
				if i%13 == 5 {
					m.Symbol(buf[:m.Samples()-1], s)
				}
				for _, v := range buf[:m.Symbol(buf[:], s)] {
					h = fnv(h, v)
					if d := int32(v) - prev; d > 40000 || d < -40000 {
						t.Fatalf("level %d offset %d: the output jumps from %d to %d", level, offset, prev, v)
					}
					prev = int32(v)
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
		{"audio 24k 16000 high", audio(Table24k(), 16000, edgeOffset(t, Table24k(), 16000, 1)), 0xcf27e341},
		{"audio 48k 16000 low", audio(Table48k(), 16000, edgeOffset(t, Table48k(), 16000, -1)), 0x6797c4e6},
		{"audio 24k 1 high", audio(Table24k(), 1, edgeOffset(t, Table24k(), 1, 1)), 0xe5842967},
		{"audio 48k 1 low", audio(Table48k(), 1, edgeOffset(t, Table48k(), 1, -1)), 0x136d0e77},
		{"iq 48k high step", iq(48000, 11999, 1, func(i int) bool { return i%9 < 5 }), 0x763bff13},
		{"iq 192k low ramp", iq(192000, -47999, 1<<15, func(i int) bool { return i < 2000 }), 0xc3526f81},
		{"iq 96k every symbol", iq(96000, 23999, 7, func(i int) bool { return i&1 == 0 }), 0x4da77038},
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
