//go:build !nocodec2 && !tinygo

package codec2

import (
	"encoding/binary"
	"errors"
	"math/rand/v2"
	"slices"
	"testing"
)

const (
	maxRaw     = 9
	maxPattern = 128
)

func fuzzPCMPool() [][]int16 {
	rng := rand.New(rand.NewPCG(0xC2, 0xF2))
	noise := make([]int16, FrameSamples)
	for i := range noise {
		noise[i] = int16(rng.Uint32())
	}
	return [][]int16{make([]int16, FrameSamples), voiced(1), noise}
}

func fuzzPCM(pool [][]int16, base uint8, pattern []byte, n, short uint8) ([]int16, int) {
	pcm := slices.Clone(pool[int(base)%len(pool)])
	pattern = pattern[:min(len(pattern), maxPattern)]
	if m := len(pattern) / 2; m > 0 {
		for i := range min(int(n), FrameSamples) {
			pcm[i] = int16(binary.LittleEndian.Uint16(pattern[2*(i%m):]))
		}
	}
	return pcm, int(short) % FrameSamples
}

func FuzzCodec2(f *testing.F) {
	f.Add([]byte{0xB9, 0xE8, 0x81, 0x52, 0x61, 0x73, 0x00, 0x2A, 0x6B}, uint8(1), []byte{}, uint8(0), uint8(0))
	f.Add(make([]byte, 9), uint8(0), []byte{}, uint8(0), uint8(0))
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, uint8(0), []byte{0x7F, 0xFF, 0x80, 0x00}, uint8(2), uint8(4))

	pool := fuzzPCMPool()
	f.Fuzz(func(t *testing.T, raw []byte, base uint8, pattern []byte, n, short uint8) {
		var slot [9]byte
		copy(slot[:], raw[:min(len(raw), maxRaw)])
		pcm, k := fuzzPCM(pool, base, pattern, n, short)
		auto := newCodec(t, Config{})
		for range 2 {
			if out, _, err := auto.Decode(nil, slot); err != nil || len(out) != FrameSamples {
				t.Fatalf("ModeAuto: Decode(%x) = %d samples, %v", slot, len(out), err)
			}
		}
		if m := auto.DecodeMode(); m != ModeAuto {
			t.Fatalf("ModeAuto: DecodeMode %d after 2 frames", m)
		}
		for _, mode := range allModes() {
			nb := mode.frameBytes()
			a := newCodec(t, Config{Mode: mode})
			if out, _, err := a.Decode(nil, slot); err != nil || len(out) != FrameSamples {
				t.Fatalf("mode %d: Decode(%x) = %d samples, %v", mode, slot, len(out), err)
			}
			if out, _, err := a.Decode(make([]int16, 3), slot); err != nil || len(out) != 3+a.FrameSamples() {
				t.Fatalf("mode %d: Decode(%x) = %d samples, %v", mode, slot, len(out), err)
			}

			enc, err := a.Encode(pcm)
			if err != nil || slices.ContainsFunc(enc[nb:], func(b byte) bool { return b != 0 }) {
				t.Fatalf("mode %d: Encode = %x, %v", mode, enc, err)
			}
			if _, err := a.Encode(pcm[:k]); !errors.Is(err, ErrFrameLength) {
				t.Fatalf("mode %d: Encode of %d samples: %v", mode, k, err)
			}
		}
	})
}
