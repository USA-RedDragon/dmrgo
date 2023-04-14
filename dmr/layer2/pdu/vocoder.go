package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/vocoder"
)

type Vocoder struct {
	bits   [216]byte
	Frames [3]vocoder.VocoderFrame
}

func (vc *Vocoder) ToString() string {
	ret := fmt.Sprintf("{ Frames: [")
	for i, frame := range vc.Frames {
		ret += fmt.Sprintf("%s", frame.ToString())
		if i < 2 {
			ret += fmt.Sprintf(", ")
		}
	}
	ret += fmt.Sprintf("] }")
	return ret
}

func NewVocoderFromBits(bits [216]byte) Vocoder {
	vc := Vocoder{
		Frames: getFrames(bits),
		bits:   bits,
	}

	return vc
}

func getFrames(bits [216]byte) [3]vocoder.VocoderFrame {
	var frames [3]vocoder.VocoderFrame

	for i := 0; i < 3; i++ {
		var frameBits [72]byte
		copy(frameBits[:], bits[i*72:(i+1)*72])
		frames[i] = *vocoder.NewVocoderFrameFromBits(frameBits)
	}

	return frames
}
