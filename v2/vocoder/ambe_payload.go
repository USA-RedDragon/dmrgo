package vocoder

import "github.com/USA-RedDragon/dmrgo/v2/bit"

// The Motorola IPSC voice burst carries the three vocoder frames of a DMR
// voice burst without their FEC in a 19-byte AMBE payload, bytes 33 to 51 of
// the IPSC packet (AMBE_DATA in DMRlink documents/voice_burst_decoding.txt).
// Frame k fills bits 50k to 50k+48, most significant bit of byte 0 first.
// Bits 49, 99 and 149 to 151 carry no frame bits.

// PackAMBEVoice packs the three frames of a voice burst into the 19-byte
// IPSC AMBE payload. The bits that carry no frame bits are 0.
func PackAMBEVoice(frames [3]AMBEFrame) [19]byte {
	var bits [152]bit.Bit
	for k, f := range frames {
		copy(bits[k*50:k*50+49], f[:])
	}
	var data [19]byte
	copy(data[:], bit.Pack(bits[:]))
	return data
}

// UnpackAMBEVoice is the reverse of PackAMBEVoice. It ignores the bits that
// carry no frame bits.
func UnpackAMBEVoice(data [19]byte) [3]AMBEFrame {
	bits := bit.Unpack(data[:])
	var frames [3]AMBEFrame
	for k := range frames {
		copy(frames[k][:], bits[k*50:k*50+49])
	}
	return frames
}
