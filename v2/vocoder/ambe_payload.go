package vocoder

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
)

// The Motorola IPSC voice burst carries the three vocoder frames of a DMR
// voice burst without their FEC in a 19-byte AMBE payload, bytes 33 to 51 of
// the IPSC packet (AMBE_DATA in DMRlink documents/voice_burst_decoding.txt).
// Frame k fills bits 50k to 50k+48, most significant bit of byte 0 first.
// Bit 49 is set when frame 2 was bad on receive and bit 99 when frame 3 was
// bad. Bits 149 to 151 are not used.

// Bits of the IPSC AMBE payload that flag frames 2 and 3 as bad.
const (
	ambeBad2Bit = 49
	ambeBad3Bit = 99
)

// PackAMBEVoice packs the three frames of a voice burst into the 19-byte
// IPSC AMBE payload. The bits that carry no frame bits are 0. It does not
// allocate.
func PackAMBEVoice(frames [3]AMBEFrame) [19]byte {
	var frameBits [152]bit.Bit
	for k, f := range frames {
		copy(frameBits[k*50:k*50+49], f[:])
	}
	var data [19]byte
	bits.Pack(data[:], 0, frameBits[:])
	return data
}

// UnpackAMBEVoice is the reverse of PackAMBEVoice. It ignores the bits that
// carry no frame bits. It does not allocate.
func UnpackAMBEVoice(data [19]byte) [3]AMBEFrame {
	var frames [3]AMBEFrame
	for k := range frames {
		for i := range frames[k] {
			n := k*50 + i
			frames[k][i] = bit.Bit(data[n/8] >> (7 - n%8) & 1)
		}
	}
	return frames
}

// PackAMBEVoiceFlags is PackAMBEVoice that also sets bit 49 when bad[1] is
// true and bit 99 when bad[2] is true, to flag frames 2 and 3 as bad. It
// ignores bad[0]: the bad flag of frame 1 is in the IPSC packet header, not
// in the AMBE payload. It does not allocate.
func PackAMBEVoiceFlags(frames [3]AMBEFrame, bad [3]bool) [19]byte {
	data := PackAMBEVoice(frames)
	if bad[1] {
		data[ambeBad2Bit/8] |= 0x80 >> (ambeBad2Bit % 8)
	}
	if bad[2] {
		data[ambeBad3Bit/8] |= 0x80 >> (ambeBad3Bit % 8)
	}
	return data
}

// UnpackAMBEVoiceFlags is the reverse of PackAMBEVoiceFlags. bad[1] and
// bad[2] report bits 49 and 99. bad[0] is always false, because the bad flag
// of frame 1 is in the IPSC packet header. It does not allocate.
func UnpackAMBEVoiceFlags(data [19]byte) (frames [3]AMBEFrame, bad [3]bool) {
	bad[1] = data[ambeBad2Bit/8]&(0x80>>(ambeBad2Bit%8)) != 0
	bad[2] = data[ambeBad3Bit/8]&(0x80>>(ambeBad3Bit%8)) != 0
	return UnpackAMBEVoice(data), bad
}
