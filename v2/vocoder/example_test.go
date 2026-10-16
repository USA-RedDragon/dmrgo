package vocoder_test

import (
	"errors"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// lsbCodec is a toy codec that sends the low bit of the first 72 samples.
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

func (lsbCodec) Decode(slot [9]byte) ([]int16, error) {
	pcm := make([]int16, 160)
	for i := range 72 {
		pcm[i] = int16(slot[i/8] >> (7 - i%8) & 1)
	}
	return pcm, nil
}

func (lsbCodec) Silence() [9]byte        { return [9]byte{} }
func (lsbCodec) FID() enums.FeatureSetID { return enums.FIDStandardized }

// ExampleNew registers a toy codec, opens it by name, and shows the error
// for an unknown name.
func ExampleNew() {
	err := vocoder.Register("example-lsb", func() (vocoder.Codec, error) { return lsbCodec{}, nil })
	if err != nil && !errors.Is(err, vocoder.ErrDuplicateCodec) {
		fmt.Println(err)
		return
	}

	c, err := vocoder.New("example-lsb")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer c.Close()

	pcm := make([]int16, c.FrameSamples())
	pcm[0], pcm[2] = 1, 1
	slot, _ := c.Encode(pcm)
	out, _ := c.Decode(slot)
	fmt.Println(c.FID(), out[:4])

	_, err = vocoder.New("no-such-codec")
	fmt.Println(err)
	// Output:
	// Standardized FID [1 0 1 0]
	// vocoder: unknown codec
}

// ExampleDecodeAMBE72 corrects a bit error in an AMBE+2 silence slot.
func ExampleDecodeAMBE72() {
	slot := vocoder.AMBESilenceSlot()
	slot[0] ^= 0x80 >> 5
	frame, res := vocoder.DecodeAMBE72(slot)
	fmt.Println(frame == vocoder.AMBESilence(), res.ErrorsCorrected)
	// Output:
	// true 1
}
