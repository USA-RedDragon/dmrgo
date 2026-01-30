package pdu

import (
	"github.com/USA-RedDragon/dmrgo/dmr/vocoder"
)

type Vocoder struct {
	bits   [216]byte
	Frames [3]vocoder.VocoderFrame
}

func (vc *Vocoder) ToString() string {
	ret := "{ Frames: ["
	for i, frame := range vc.Frames {
		ret += frame.ToString()
		if i < 2 {
			ret += ", "
		}
	}
	ret += "] }"
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
		frames[i] = vocoder.NewVocoderFrameFromBits(frameBits)
	}

	return frames
}

// Encode returns the 216 bits of the vocoder PDU.
func (vc *Vocoder) Encode() [216]byte {
	var bits [216]byte
	for i := 0; i < 3; i++ {
		frameBits := vc.Frames[i].Encode()
		copy(bits[i*72:(i+1)*72], frameBits[:])
	}
	return bits
}

// CorrectedErrors returns the total number of corrected bit errors in the vocoder frames.
func (vc *Vocoder) CorrectedErrors() int {
	var count int
	for _, f := range vc.Frames {
		count += f.CorrectedErrors
	}
	return count
}

// Uncorrectable returns true if any of the vocoder frames were uncorrectable.
func (vc *Vocoder) Uncorrectable() bool {
	for _, f := range vc.Frames {
		if f.Uncorrectable {
			return true
		}
	}
	return false
}

// HasError returns true if any error correction was performed or if the data is uncorrectable.
func (vc *Vocoder) HasError() bool {
	return vc.CorrectedErrors() > 0 || vc.Uncorrectable()
}
