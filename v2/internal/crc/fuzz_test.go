package crc_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	ref "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/crc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

// crcSeed is one seed of FuzzCRC.
type crcSeed struct {
	data  []byte
	nbits uint16
	flip  uint16
	wrong uint8
}

// crcSeeds returns the seeds of FuzzCRC.
func crcSeeds() []crcSeed {
	return []crcSeed{
		{[]byte{0x00, 0x2e, 0x44, 0x4d, 0x52, 0x47, 0x4f, 0x20, 0x54, 0x45, 0x53, 0x54}, uint16(80), uint16(3), uint8(5)},
		{[]byte{0x02, 0x00, 0x00, 0x00, 0x09, 0x00, 0x30, 0x39, 0x00, 0x00}, uint16(72), uint16(71), uint8(1)},
		{[]byte{0xFF}, uint16(7), uint16(7), uint8(0)},
		{[]byte{}, uint16(0), uint16(0), uint8(0)},
	}
}

// checkCRC checks the properties of FuzzCRC for one input.
func checkCRC(t testing.TB, data []byte, nbits, flip uint16, wrong uint8) {
	t.Helper()
	n := int(nbits) % (8*len(data) + 1)
	bits := unpacked.Unpack(data)

	c7, c8, c9 := crc.CRC7(data, n), crc.CRC8(data, n), crc.CRC9(data, n)
	cc, c32 := crc.CCITT(data), crc.CRC32(data)
	if c7 != ref.CRC7(bits[:n]) || c8 != ref.CRC8(bits[:n]) || c9 != ref.CRC9(bits[:n]) || cc != ref.CCITT(data) || c32 != ref.CRC32(data) {
		t.Fatalf("CRCs of %x over %d bits: %#x %#x %#x %#x %#x, reference %#x %#x %#x %#x %#x", data, n,
			c7, c8, c9, cc, c32, ref.CRC7(bits[:n]), ref.CRC8(bits[:n]), ref.CRC9(bits[:n]), ref.CCITT(data), ref.CRC32(data))
	}
	var nine [9]byte
	copy(nine[:], data)
	if crc.Checksum5(nine) != ref.Checksum5(nine) || crc.Checksum5(nine) > 30 {
		t.Fatalf("Checksum5(%x) = %d, reference %d", nine, crc.Checksum5(nine), ref.Checksum5(nine))
	}

	w := uint16(wrong) | 1
	if !crc.CheckCRC7(data, n, c7|0x80) || crc.CheckCRC7(data, n, c7^uint8(w&0x7F|1)) ||
		!crc.CheckCRC8(data, n, c8) || crc.CheckCRC8(data, n, c8^uint8(w)) ||
		!crc.CheckCRC9(data, n, c9|0xFE00) || crc.CheckCRC9(data, n, c9^(w<<1&0x1FF|1)) ||
		!crc.CheckCCITT(data, cc) || crc.CheckCCITT(data, cc^w) ||
		!crc.CheckCRC32(data, c32) || crc.CheckCRC32(data, c32^uint32(w)<<int(flip%24)) ||
		!crc.CheckChecksum5(nine, crc.Checksum5(nine)) || crc.CheckChecksum5(nine, crc.Checksum5(nine)^uint8(w&0x1F)) {
		t.Fatalf("a check of %x over %d bits accepts a wrong value or rejects its own", data, n)
	}

	if len(data) > 0 {
		p := int(flip) % (8 * len(data))
		flipped := append([]byte(nil), data...)
		flipped[p/8] ^= 0x80 >> (p % 8)
		covered := p < n
		if (crc.CRC7(flipped, n) != c7) != covered || (crc.CRC8(flipped, n) != c8) != covered || (crc.CRC9(flipped, n) != c9) != covered {
			t.Fatalf("flipping bit %d of %x changes the CRCs over %d bits: %v %v %v", p, data, n,
				crc.CRC7(flipped, n) != c7, crc.CRC8(flipped, n) != c8, crc.CRC9(flipped, n) != c9)
		}
		if crc.CCITT(flipped) == cc || crc.CRC32(flipped) == c32 {
			t.Fatalf("flipping bit %d of %x does not change CRC-CCITT or CRC-32", p, data)
		}
		k := int(flip) % (len(data) + 1)
		if got := ^crc.CCITTUpdate(crc.CCITTUpdate(0, data[:k]), data[k:]); got != cc {
			t.Fatalf("CRC-CCITT of %x split at %d = %#x, want %#x", data, k, got, cc)
		}
	}
}

// FuzzCRC computes each CRC and the 5-bit checksum of fuzzed data, over
// the first n bits for the bit-length CRCs. Each agrees with the reference
// codec, accepts its own value and rejects every other value of its width,
// changes when one bit it covers flips, ignores the bits after n, and
// CRC-CCITT gives the same value over the data split at any byte.
func FuzzCRC(f *testing.F) {
	for _, s := range crcSeeds() {
		f.Add(s.data, s.nbits, s.flip, s.wrong)
	}
	f.Fuzz(func(t *testing.T, data []byte, nbits, flip uint16, wrong uint8) {
		checkCRC(t, data, nbits, flip, wrong)
	})
}
