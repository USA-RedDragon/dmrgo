package vocoder

import (
	"encoding/hex"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/bit"
	"github.com/USA-RedDragon/dmrgo/dmr/fec/golay"
	"github.com/USA-RedDragon/dmrgo/dmr/fec/prng"
)

type VocoderFrame struct {
	DecodedBits     [49]bit.Bit
	CorrectedErrors int
	Uncorrectable   bool
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

	a := golay.Golay_24_12_8_EncodingTable[aOrig]

	// The PRNG
	p := prng.PRNG_TABLE[aOrig] >> 1

	b := golay.Golay_23_12_7_EncodingTable[bOrig] >> 1
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

//nolint:gochecknoglobals // static mapping tables
var aTable = []int{0, 4, 8, 12, 16, 20, 24, 28, 32, 36, 40, 44,
	48, 52, 56, 60, 64, 68, 1, 5, 9, 13, 17, 21}

//nolint:gochecknoglobals // static mapping tables
var bTable = []int{25, 29, 33, 37, 41, 45, 49, 53, 57, 61, 65, 69,
	2, 6, 10, 14, 18, 22, 26, 30, 34, 38, 42}

//nolint:gochecknoglobals // static mapping tables
var cTable = []int{46, 50, 54, 58, 62, 66, 70, 3, 7, 11, 15, 19,
	23, 27, 31, 35, 39, 43, 47, 51, 55, 59, 63, 67, 71}

func NewVocoderFrameFromBits(bits [72]bit.Bit) VocoderFrame {
	var ambe49 [49]bit.Bit
	totalErrors := 0
	uncorrectable := false

	var a uint32 = 0
	var MASK uint32 = 0x800000

	for i := 0; i < 24; i, MASK = i+1, MASK>>1 {
		aPos := aTable[i]
		if bits[aPos] == 1 {
			a |= MASK
		}
	}

	// shift right by 1 to make it 23 bits for PRNG? No, PRNG_TABLE index is 12 bits.
	// Golay 24,12,8: 24 bits received. 12 data bits.
	// Decode 'a'
	dataA, errsA, failA := golay.DecodeGolay24128(a)
	totalErrors += errsA
	if failA {
		uncorrectable = true
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

	// Decrypt 'b' using 'a' before decoding? Or decode then decrypt?
	// The Encode function does:
	// a = Table[aOrig]
	// p = PRNG[aOrig] >> 1
	// b = (Table[bOrig] >> 1) ^ p
	// So b contains PRNG[aOrig].
	// To decode b, we must first XOR with p.
	// We need aOrig for that. We have 'a' (which is the corrected aOrig).

	// The PRNG
	// 'a' here is the 12-bit data (aOrig).
	b ^= (prng.PRNG_TABLE[a] >> 1)

	// Check/Decode 'b'
	// 'b' is now (Table[bOrig] >> 1). It is 23 bits.
	// Encode shifted right by 1.
	// Golay_23 is 23 bits.
	// DecodeGolay23127 expects 23 bits.
	// Is the result of Encode() 23 bits in the lower part?
	// In Encode(): b = ... >> 1. 23 bits.
	// Stored in ambe72 using bTable which has 23 entries.
	// So 'b' assembled here is exactly the 23 bits.

	dataB, errsB, failB := golay.DecodeGolay23127(b)
	totalErrors += errsB
	if failB {
		uncorrectable = true
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
	// 'c' is 25 bits uncoded. No FEC.
	// Just use it. But in correct format for loop below?
	// Loop below expects 'c' to be shifted?
	// In Encode: if (cOrig & MASK) != 0. MASK=0x1000000 (bit 24).
	// cOrig is constructed from ambe49.
	// Here 'c' is read from bits.
	// We don't need to shift 'c', it should already be aligned if read correctly.
	// Wait, loop below uses MASK=0x1000000 for c.

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
		DecodedBits:     ambe49,
		CorrectedErrors: totalErrors,
		Uncorrectable:   uncorrectable,
	}
}

// PackAMBEVoice packs 3 FEC-decoded VocoderFrames (each 49 bits) into
// a 19-byte (152-bit) AMBE payload.
//
// Layout: frame0[49 bits] + 1 separator(0) + frame1[49 bits] + 1 separator(0) + frame2[49 bits] + 3 padding(0)
func PackAMBEVoice(frames [3]VocoderFrame) [19]byte {
	var bits [152]bit.Bit

	// Frame 0: bits 0-48
	for i := 0; i < 49; i++ {
		bits[i] = frames[0].DecodedBits[i]
	}
	// Bit 49: separator (0)

	// Frame 1: bits 50-98
	for i := 0; i < 49; i++ {
		bits[50+i] = frames[1].DecodedBits[i]
	}
	// Bit 99: separator (0)

	// Frame 2: bits 100-148
	for i := 0; i < 49; i++ {
		bits[100+i] = frames[2].DecodedBits[i]
	}
	// Bits 149-151: padding (0)

	// Pack bits into bytes (MSB first)
	var data [19]byte
	for i := 0; i < 152; i++ {
		if bits[i] == 1 {
			data[i/8] |= 1 << (7 - (i % 8))
		}
	}

	return data
}

// UnpackAMBEVoice unpacks 19 bytes of AMBE payload into 3 VocoderFrames (49 decoded bits each).
// This is the reverse of PackAMBEVoice.
func UnpackAMBEVoice(data [19]byte) [3]VocoderFrame {
	// Unpack bytes to bits (MSB first)
	var bits [152]bit.Bit
	for i := 0; i < 152; i++ {
		if (data[i/8]>>(7-(i%8)))&1 == 1 {
			bits[i] = 1
		}
	}

	var frames [3]VocoderFrame

	// Frame 0: bits 0-48
	for i := 0; i < 49; i++ {
		frames[0].DecodedBits[i] = bits[i]
	}

	// Frame 1: bits 50-98
	for i := 0; i < 49; i++ {
		frames[1].DecodedBits[i] = bits[50+i]
	}

	// Frame 2: bits 100-148
	for i := 0; i < 49; i++ {
		frames[2].DecodedBits[i] = bits[100+i]
	}

	return frames
}
