//go:build !nocodec2 && !tinygo

package codec2

import "testing"

func TestAllocsAutoDecode(t *testing.T) {
	c := newCodec(t, Config{})
	slots := encodeAll(t, newCodec(t, Config{Mode: Mode3200}), voiced(4))
	dst := make([]int16, 0, FrameSamples)
	i := 0
	allocs := testing.AllocsPerRun(100, func() {
		dst, _ = c.Decode(dst[:0], slots[i%len(slots)])
		i++
		if i%7 == 0 {
			c.Reset()
		}
	})
	if allocs != 0 {
		t.Errorf("Decode in ModeAuto: %.1f allocations", allocs)
	}
}
