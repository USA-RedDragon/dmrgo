//go:build cgo && !nocodec2 && !tinygo

package codec2

import (
	"encoding/binary"
	"errors"
	"slices"
	"testing"
)

// FuzzCodec2 decodes a fuzzed slot and encodes fuzzed samples with
// libcodec2 in each mode. Decode returns FrameSamples samples without error,
// Encode leaves the bits after the Codec2 frame 0, and Encode rejects a
// frame of another length. libcodec2 draws the noise of its synthesis from
// one generator for all its states, so two decodes of the same slot can
// differ.
func FuzzCodec2(f *testing.F) {
	f.Add([]byte{0xB9, 0xE8, 0x81, 0x52, 0x61, 0x73, 0x00, 0x2A, 0x6B}, voicedBytes())
	f.Add(make([]byte, 9), []byte{})
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, []byte{0x7F, 0xFF, 0x80, 0x00})

	f.Fuzz(func(t *testing.T, raw, samples []byte) {
		var slot [9]byte
		copy(slot[:], raw)
		pcm := make([]int16, FrameSamples)
		for i := range pcm {
			if 2*i+1 < len(samples) {
				pcm[i] = int16(binary.LittleEndian.Uint16(samples[2*i:]))
			}
		}
		for _, mode := range allModes() {
			n := mode.frameBytes()
			a := newCodec(t, Config{Mode: mode})
			if out, err := a.Decode(slot); err != nil || len(out) != FrameSamples {
				t.Fatalf("mode %d: Decode(%x) = %d samples, %v", mode, slot, len(out), err)
			}

			enc, err := a.Encode(pcm)
			if err != nil || slices.ContainsFunc(enc[n:], func(b byte) bool { return b != 0 }) {
				t.Fatalf("mode %d: Encode = %x, %v", mode, enc, err)
			}
			if _, err := a.Encode(pcm[:len(samples)%FrameSamples]); !errors.Is(err, ErrFrameLength) {
				t.Fatalf("mode %d: Encode of %d samples: %v", mode, len(samples)%FrameSamples, err)
			}
		}
	})
}

// voicedBytes returns one frame of voiced as little-endian bytes.
func voicedBytes() []byte {
	pcm := voiced(1)
	out := make([]byte, 2*len(pcm))
	for i, s := range pcm {
		binary.LittleEndian.PutUint16(out[2*i:], uint16(s))
	}
	return out
}
