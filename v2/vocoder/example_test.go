package vocoder_test

import (
	"errors"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// lsbCodec is a toy codec that sends the low bit of the first 72 samples.
type lsbCodec struct{}

func (lsbCodec) Close() error      { return nil }
func (lsbCodec) FrameSamples() int { return 160 }

func (lsbCodec) Encode(pcm []int16) ([72]bit.Bit, error) {
	var s [72]bit.Bit
	for i := range s {
		s[i] = bit.Bit(pcm[i] & 1)
	}
	return s, nil
}

func (lsbCodec) Decode(slot [72]bit.Bit) ([]int16, error) {
	pcm := make([]int16, 160)
	for i, b := range slot {
		pcm[i] = int16(b)
	}
	return pcm, nil
}

func (lsbCodec) Silence() [72]bit.Bit    { return [72]bit.Bit{} }
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
	// vocoder: unknown codec: "no-such-codec"
}

// ExampleDecodeAMBE72 corrects a bit error in an AMBE+2 silence slot.
func ExampleDecodeAMBE72() {
	slot := vocoder.EncodeAMBE72(vocoder.AMBESilence())
	slot[5] ^= 1
	frame, res := vocoder.DecodeAMBE72(slot)
	fmt.Println(frame == vocoder.AMBESilence(), res.ErrorsCorrected)
	// Output:
	// true 1
}
