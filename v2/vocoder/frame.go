package vocoder

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/ambe"
)

// AMBEFrame holds the 49 information bits of one AMBE+2 vocoder frame,
// without the frame FEC, packed, the first bit in the most significant bit
// of byte 0. The low 7 bits of byte 6 are padding: DecodeAMBE72 sets them to
// 0, and EncodeAMBE72 returns an error when one of them is set.
type AMBEFrame [7]byte

// AMBESilence returns the AMBE+2 frame of silence that MMDVMHost sends to
// fill a voice burst (DMR_SILENCE_DATA); EncodeAMBE72 of it is the slot
// B9E881526173002A6B.
func AMBESilence() AMBEFrame {
	return AMBEFrame{0xF8, 0x01, 0xA9, 0x9F, 0x8C, 0xE0, 0x80}
}

// AMBESilenceSlot returns the 72 on-air bits of the AMBE+2 silence frame,
// EncodeAMBE72(AMBESilence()), the slot B9E881526173002A6B. It is the
// silence slot that pads the last superframe of an AMBE+2 call.
func AMBESilenceSlot() [9]byte {
	return [9]byte{0xB9, 0xE8, 0x81, 0x52, 0x61, 0x73, 0x00, 0x2A, 0x6B}
}

// EncodeAMBE72 applies the AMBE+2 frame FEC to 49 vocoder information bits
// and returns the 72 on-air bits of one vocoder frame, packed, the first bit
// in the most significant bit of byte 0: Golay(24,12) on the first 12 bits,
// PN-scrambled Golay(23,12) on the next 12, the last 25 bits unprotected,
// interleaved into the slot. It returns a *bit.FieldRangeError when a
// padding bit of info is set.
func EncodeAMBE72(info AMBEFrame) ([9]byte, error) {
	if info[6]&0x7F != 0 {
		return [9]byte{}, errAMBEFrameRange
	}
	return ambe.Encode(info), nil
}

// errAMBEFrameRange is the error of an EncodeAMBE72 whose frame has a
// padding bit set.
var errAMBEFrameRange = &bit.FieldRangeError{Type: "vocoder", Field: "AMBEFrame", Bits: 49}

// DecodeAMBE72 reverses EncodeAMBE72 and corrects the bit errors that the
// Golay codes allow. Uncorrectable reflects only the overall parity check of
// the Golay (24,12) section A: 4 or more errors in the perfect Golay (23,12)
// section B decode to wrong bits without a flag. A caller that mutes or
// repeats frames should also test ErrorsCorrected (sections A and B).
func DecodeAMBE72(slot [9]byte) (AMBEFrame, fec.Result) {
	return ambe.Decode(slot)
}
