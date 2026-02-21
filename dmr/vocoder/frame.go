package vocoder

import (
	"encoding/hex"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/bit"
	"github.com/USA-RedDragon/dmrgo/dmr/fec"
	"github.com/USA-RedDragon/dmrgo/dmr/fec/golay"
)

type VocoderFrame struct {
	DecodedBits [49]bit.Bit
	FEC         fec.FECResult
}

func (vf *VocoderFrame) ToString() string {
	// convert DecodedBits to a byte array
	var data [7]byte
	for i := 0; i < 6; i++ {
		data[i] = byte(vf.DecodedBits[i*8])<<7 | byte(vf.DecodedBits[i*8+1])<<6 | byte(vf.DecodedBits[i*8+2])<<5 | byte(vf.DecodedBits[i*8+3])<<4 | byte(vf.DecodedBits[i*8+4])<<3 | byte(vf.DecodedBits[i*8+5])<<2 | byte(vf.DecodedBits[i*8+6])<<1 | byte(vf.DecodedBits[i*8+7])
	}
	data[6] = byte(vf.DecodedBits[48]) << 7
	return fmt.Sprintf("{ DecodedFrame: %014s }", hex.EncodeToString(data[:]))
}

// Encode takes the decoded bits and encodes them into a 216 bit frame
func (vf *VocoderFrame) Encode() [72]bit.Bit {
	var ambe72 [72]bit.Bit

	var ambe49 = vf.DecodedBits

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

	a := golay.Golay_24_12_8_table[aOrig]

	p := AMBE_SCRAMBLE_TABLE[aOrig] >> 1

	b := golay.Golay_23_12_7_table[bOrig] >> 1
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

func NewVocoderFrameFromBits(bits [72]bit.Bit) VocoderFrame {
	var ambe49 [49]bit.Bit
	result := fec.FECResult{BitsChecked: 47} // 24 (Golay 24,12,8) + 23 (Golay 23,12,7)

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
	dataA, fecA := golay.DecodeGolay24128(a)
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

	b ^= (AMBE_SCRAMBLE_TABLE[a] >> 1)

	dataB, fecB := golay.DecodeGolay23127(b)
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

	return VocoderFrame{
		DecodedBits: ambe49,
		FEC:         result,
	}
}
