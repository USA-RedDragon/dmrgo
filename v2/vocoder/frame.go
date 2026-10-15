package vocoder

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/fec/golay"
)

// VocoderFrame is one decoded AMBE+2 vocoder frame: the 49 information bits
// and the FEC result of the frame FEC. FEC.Uncorrectable reflects only
// section A; see DecodeAMBE72.
//
// dmr:fec ambe_voice
type VocoderFrame struct {
	DecodedBits [49]bit.Bit `dmr:"bits:0-48,raw"`
	FEC         fec.Result  `dmr:"-"`
}

// EncodeAMBE72 applies the AMBE+2 frame FEC to 49 vocoder information bits
// and returns the 72 on-air bits of one vocoder frame: Golay(24,12) on the
// first 12 bits, PN-scrambled Golay(23,12) on the next 12, the last 25 bits
// unprotected, interleaved into the slot.
func EncodeAMBE72(info [49]bit.Bit) [72]bit.Bit {
	bit.Mask(info[:])
	return encodeAMBE(info)
}

// DecodeAMBE72 reverses EncodeAMBE72, correcting bit errors where the Golay
// codes allow.
//
// Golay (23,12) is a perfect code: every 23-bit word is within 3 bits of
// exactly one codeword, so section B never reports Uncorrectable, and 4 or
// more bit errors there decode to wrong bits without a flag. The result's
// Uncorrectable therefore reflects only the overall parity check of section
// A's Golay (24,12). A caller that mutes or repeats frames on Uncorrectable
// should also test ErrorsCorrected, the number of corrected bits in sections
// A and B, parity bits included. mbelib, for example, repeats the last frame
// when errs2 is above 3; errs2 counts only the corrected data bits of
// sections A and B, and mbelib does not check the section A parity bit.
func DecodeAMBE72(slot [72]bit.Bit) ([49]bit.Bit, fec.Result) {
	bit.Mask(slot[:])
	return decodeAMBE(slot)
}

// encodeAMBE takes the decoded 49 bits and encodes them into a 72 bit AMBE frame.
func encodeAMBE(ambe49 [49]bit.Bit) [72]bit.Bit {
	var ambe72 [72]bit.Bit

	var aOrig uint32 = 0
	var bOrig uint32 = 0
	var cOrig uint32 = 0
	var MASK uint32 = 0x000800

	for i := 0; i < 12; i, MASK = i+1, MASK>>1 {
		n1 := i
		n2 := i + 12
		if ambe49[n1] == 1 {
			aOrig |= MASK
		}
		if ambe49[n2] == 1 {
			bOrig |= MASK
		}
	}

	MASK = 0x1000000
	for i := 0; i < 25; i, MASK = i+1, MASK>>1 {
		n := i + 24
		if ambe49[n] == 1 {
			cOrig |= MASK
		}
	}

	a := golay.Encode24128(uint16(aOrig))

	p := ambeScrambleTable[aOrig] >> 1

	b := golay.Encode23127(uint16(bOrig))
	b ^= p

	MASK = 0x800000
	for i := 0; i < 24; i, MASK = i+1, MASK>>1 {
		aPos := aTable[i]
		if (a & MASK) != 0 {
			ambe72[aPos] = 1
		} else {
			ambe72[aPos] = 0
		}
	}

	MASK = 0x400000
	for i := 0; i < 23; i, MASK = i+1, MASK>>1 {
		bPos := bTable[i]
		if (b & MASK) != 0 {
			ambe72[bPos] = 1
		} else {
			ambe72[bPos] = 0
		}
	}

	MASK = 0x1000000
	for i := 0; i < 25; i, MASK = i+1, MASK>>1 {
		cPos := cTable[i]
		if (cOrig & MASK) != 0 {
			ambe72[cPos] = 1
		} else {
			ambe72[cPos] = 0
		}
	}

	return ambe72
}

// decodeAMBE decodes a 72-bit AMBE voice frame into 49 decoded bits with FEC.
// Sections: A (24 scattered → Golay 24,12,8 → 12 bits),
// B (23 scattered → descramble → Golay 23,12,7 → 12 bits),
// C (25 scattered → 25 raw bits).
func decodeAMBE(bits [72]bit.Bit) ([49]bit.Bit, fec.Result) {
	var ambe49 [49]bit.Bit
	result := fec.Result{BitsChecked: 47} // 24 (Golay 24,12,8) + 23 (Golay 23,12,7)

	var a uint32 = 0
	var MASK uint32 = 0x800000

	for i := 0; i < 24; i, MASK = i+1, MASK>>1 {
		aPos := aTable[i]
		if bits[aPos] == 1 {
			a |= MASK
		}
	}

	// Golay 24,12,8: 24 bits received. 12 data bits.
	// Decode 'a'
	dataA, fecA := golay.Decode24128(a)
	result.ErrorsCorrected += fecA.ErrorsCorrected
	if fecA.Uncorrectable {
		result.Uncorrectable = true
	}
	// Use corrected data for 'a'
	a = uint32(dataA)

	var b uint32 = 0
	MASK = 0x400000
	for i := 0; i < 23; i, MASK = i+1, MASK>>1 {
		bPos := bTable[i]
		if bits[bPos] == 1 {
			b |= MASK
		}
	}

	b ^= (ambeScrambleTable[a] >> 1)

	dataB, fecB := golay.Decode23127(b)
	result.ErrorsCorrected += fecB.ErrorsCorrected
	if fecB.Uncorrectable {
		result.Uncorrectable = true
	}
	b = uint32(dataB)

	var c uint32 = 0
	MASK = 0x1000000
	for i := 0; i < 25; i, MASK = i+1, MASK>>1 {
		cPos := cTable[i]
		if bits[cPos] == 1 {
			c |= MASK
		}
	}

	// Reconstruct ambe49
	MASK = 0x000800
	for i := 0; i < 12; i, MASK = i+1, MASK>>1 {
		apos := i
		bpos := i + 12
		// 'a' is 12 bits data. MASK scans 12 bits.
		if (a & MASK) != 0 {
			ambe49[apos] = 1
		} else {
			ambe49[apos] = 0
		}
		// 'b' is 12 bits data.
		if (b & MASK) != 0 {
			ambe49[bpos] = 1
		} else {
			ambe49[bpos] = 0
		}
	}

	MASK = 0x1000000
	for i := 0; i < 25; i, MASK = i+1, MASK>>1 {
		cPos := i + 24
		if (c & MASK) != 0 {
			ambe49[cPos] = 1
		} else {
			ambe49[cPos] = 0
		}
	}

	return ambe49, result
}
