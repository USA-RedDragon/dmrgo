package vocoder_test

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

type lsbCodec struct{}

func (lsbCodec) Close() error      { return nil }
func (lsbCodec) FrameSamples() int { return 160 }

func (lsbCodec) Encode(pcm []int16) ([9]byte, error) {
	var s [9]byte
	for i := range 72 {
		s[i/8] |= byte(pcm[i]&1) << (7 - i%8)
	}
	return s, nil
}

func (lsbCodec) Decode(dst []int16, slot [9]byte) ([]int16, error) {
	n := len(dst)
	dst = append(dst, make([]int16, 160)...)
	for i := range 72 {
		dst[n+i] = int16(slot[i/8] >> (7 - i%8) & 1)
	}
	return dst, nil
}

func (lsbCodec) Reset() {}

func (lsbCodec) Silence() [9]byte { return [9]byte{} }
func (lsbCodec) FID() enums.FID   { return enums.FIDStandardized }

// ExampleCodec implements Codec with a toy codec that keeps the lowest bit
// of the first 72 samples, and runs one frame through it.
func ExampleCodec() {
	var c vocoder.Codec = lsbCodec{}
	defer c.Close()

	pcm := make([]int16, c.FrameSamples())
	pcm[0], pcm[2] = 1, 1
	slot, _ := c.Encode(pcm)
	out, _ := c.Decode(nil, slot)
	fmt.Println(c.FID(), out[:4])
	// Output:
	// Standardized FID [1 0 1 0]
}

// ExampleDecodeAMBE72 corrects one bit error in the silence frame.
func ExampleDecodeAMBE72() {
	slot := vocoder.AMBESilenceSlot()
	slot[0] ^= 0x80 >> 5
	frame, res := vocoder.DecodeAMBE72(slot)
	fmt.Println(frame == vocoder.AMBESilence(), res.ErrorsCorrected)
	// Output:
	// true 1
}
