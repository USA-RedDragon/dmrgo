//go:build !nocodec2 && !tinygo && !(js && wasm)

package codec2

import "testing"

func TestAllocsAutoDecode(t *testing.T) {
	for i, m := range allocModules(t) {
		c := newCodecOf(t, Config{WASM: m})
		slots := encodeAll(t, newCodecOf(t, Config{Mode: Mode3200, WASM: m}), voiced(4))
		dst := make([]int16, 0, FrameSamples)
		n := 0
		allocs := testing.AllocsPerRun(100, func() {
			dst, _ = c.Decode(dst[:0], slots[n%len(slots)])
			n++
			if n%7 == 0 {
				c.Reset()
			}
		})
		if allocs != 0 {
			t.Errorf("module %d: Decode in ModeAuto: %.1f allocations", i, allocs)
		}
	}
}

func TestAllocsEncode(t *testing.T) {
	pcm := voiced(4)
	for i, m := range allocModules(t) {
		for _, mode := range []Mode{Mode2400, Mode3200} {
			c := newCodecOf(t, Config{Mode: mode, WASM: m})
			n := 0
			allocs := testing.AllocsPerRun(100, func() {
				off := (n % 4) * FrameSamples
				_, _ = c.Encode(pcm[off : off+FrameSamples])
				n++
			})
			if allocs != 0 {
				t.Errorf("module %d: Encode in mode %d: %.1f allocations", i, mode, allocs)
			}
		}
	}
}
