package vocoder

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/ambe"
)

// AMBEFrame holds the 49 information bits of one AMBE+2 vocoder frame,
// without the frame FEC.
type AMBEFrame [49]bit.Bit

// AMBESilence returns the AMBE+2 frame of silence that MMDVMHost sends to
// fill a voice burst (DMR_SILENCE_DATA); EncodeAMBE72 of it is the slot
// B9E881526173002A6B.
func AMBESilence() AMBEFrame {
	return AMBEFrame{
		1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 0, 1, 0, 1, 0, 0, 1, 1,
		0, 0, 1, 1, 1, 1, 1, 1, 0, 0, 0, 1, 1, 0, 0, 1, 1, 1, 0, 0, 0, 0, 0, 1,
	}
}

// AMBESilenceSlot returns the 72 on-air bits of the AMBE+2 silence frame,
// EncodeAMBE72(AMBESilence()), the slot B9E881526173002A6B. It is the
// silence slot that pads the last superframe of an AMBE+2 call.
func AMBESilenceSlot() [72]bit.Bit {
	return [72]bit.Bit{
		1, 0, 1, 1, 1, 0, 0, 1, 1, 1, 1, 0, 1, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 1, 0, 1, 0, 1, 0, 0, 1, 0, 0, 1, 1, 0,
		0, 0, 0, 1, 0, 1, 1, 1, 0, 0, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 1, 0, 1, 0, 0, 1, 1, 0, 1, 0, 1, 1,
	}
}

// EncodeAMBE72 applies the AMBE+2 frame FEC to 49 vocoder information bits
// and returns the 72 on-air bits of one vocoder frame: Golay(24,12) on the
// first 12 bits, PN-scrambled Golay(23,12) on the next 12, the last 25 bits
// unprotected, interleaved into the slot.
func EncodeAMBE72(info AMBEFrame) [72]bit.Bit {
	var frame [7]byte
	bits.Pack(frame[:], 0, info[:])
	packed := ambe.Encode(frame)
	var slot [72]bit.Bit
	bits.Unpack(slot[:], packed[:], 0)
	return slot
}

// DecodeAMBE72 reverses EncodeAMBE72 and corrects the bit errors that the
// Golay codes allow. Uncorrectable reflects only the overall parity check of
// the Golay (24,12) section A: 4 or more errors in the perfect Golay (23,12)
// section B decode to wrong bits without a flag. A caller that mutes or
// repeats frames should also test ErrorsCorrected (sections A and B).
func DecodeAMBE72(slot [72]bit.Bit) (AMBEFrame, fec.Result) {
	var packed [9]byte
	bits.Pack(packed[:], 0, slot[:])
	frame, res := ambe.Decode(packed)
	var info AMBEFrame
	bits.Unpack(info[:], frame[:], 0)
	return info, res
}
