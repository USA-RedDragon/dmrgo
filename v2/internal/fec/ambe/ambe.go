// Package ambe applies the AMBE+2 frame FEC of a 72-bit DMR vocoder slot:
// Golay(24,12) on the first 12 of the 49 vocoder bits, PN-scrambled
// Golay(23,12) on the next 12, and the last 25 bits unprotected, interleaved
// into the slot. A slot is packed most significant bit first in 9 bytes, and
// a frame of 49 bits in 7 bytes. (ETSI TS 102 361-1 clause 6.1)
package ambe

import (
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/golay"
)

func columns(slot *[9]byte) (c0, c1, c2, c3 uint32) {
	for _, b := range slot {
		x := uint32(b)
		c0 = c0<<2 | x>>6&2 | x>>3&1
		c1 = c1<<2 | x>>5&2 | x>>2&1
		c2 = c2<<2 | x>>4&2 | x>>1&1
		c3 = c3<<2 | x>>3&2 | x&1
	}
	return c0, c1, c2, c3
}

// Decode reverses Encode and corrects the bit errors that the Golay codes
// allow. BitsChecked is 47. Uncorrectable reflects only the overall parity
// check of the Golay(24,12) section A: 4 or more errors in the perfect
// Golay(23,12) section B decode to wrong bits without a flag.
func Decode(slot [9]byte) ([7]byte, fec.Result) {
	c0, c1, c2, c3 := columns(&slot)
	a, resA := golay.Decode24128(c0<<6 | c1>>12)
	b, resB := golay.Decode23127((c1&0xFFF)<<11 | c2>>7 ^ ambeScrambleTable[a]>>1)
	c := (c2&0x7F)<<18 | c3

	result := resA.Combine(resB)
	result.BitsChecked = 47
	v := uint64(a)<<37 | uint64(b)<<25 | uint64(c)
	var frame [7]byte
	for i := range 6 {
		frame[i] = byte(v >> (41 - 8*i) & 0xFF)
	}
	frame[6] = byte(v&1) << 7
	return frame, result
}

// Encode returns the 72-bit slot of the 49 vocoder bits in frame. The low 7
// bits of frame[6] are ignored.
func Encode(frame [7]byte) [9]byte {
	var v uint64
	for _, b := range frame {
		v = v<<8 | uint64(b)
	}
	v >>= 7
	a := uint32(v >> 37 & 0xFFF)
	b := uint32(v >> 25 & 0xFFF)
	c := uint32(v & 0x1FFFFFF)

	ca := golay.Encode24128(uint16(a))
	cb := golay.Encode23127(uint16(b)) ^ ambeScrambleTable[a]>>1
	c0 := ca >> 6
	c1 := (ca&0x3F)<<12 | cb>>11
	c2 := (cb&0x7FF)<<7 | c>>18
	c3 := c & 0x3FFFF

	var slot [9]byte
	for i := range slot {
		sh := 16 - 2*i
		slot[i] = byte((c0>>sh&2<<6 | c1>>sh&2<<5 | c2>>sh&2<<4 | c3>>sh&2<<3 |
			c0>>sh&1<<3 | c1>>sh&1<<2 | c2>>sh&1<<1 | c3>>sh&1) & 0xFF)
	}
	return slot
}

// Regenerate decodes slot and encodes it again, so that the bit errors that
// Decode corrects are not passed on, as MMDVMHost CAMBEFEC::regenerateDMR
// does. It returns slot unchanged and false when section A is
// uncorrectable, or when 6 or more errors with 2 or more in section A make
// regenerateDMR mute the frame (AMBEFEC.cpp lines 843-883, g4klx/MMDVMHost
// 9384d683).
func Regenerate(slot [9]byte) ([9]byte, bool) {
	frame, res := Decode(slot)
	if res.Uncorrectable {
		return slot, false
	}
	out := Encode(frame)
	o0, o1, _, _ := columns(&out)
	s0, s1, _, _ := columns(&slot)
	if bits.OnesCount32((o0<<6|o1>>12)^(s0<<6|s1>>12)) >= 2 && res.ErrorsCorrected >= 6 {
		return slot, false
	}
	return out, true
}
