package ambe

import (
	"math/bits"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
	refambe "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/ambe"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

// frameBits returns the 49 vocoder bits of frame, the first most
// significant.
func frameBits(frame [7]byte) uint64 {
	var v uint64
	for _, b := range frame {
		v = v<<8 | uint64(b)
	}
	return v >> 7
}

// checkAMBE checks the properties of FuzzAMBE for one input.
func checkAMBE(t testing.TB, frame [7]byte, mask [9]byte) {
	t.Helper()
	frame[6] &= 0x80

	slot := Encode(frame)
	var info [49]unpacked.Bit
	copy(info[:], unpacked.Unpack(frame[:]))
	if want := refambe.Encode(info); slot != [9]byte(unpacked.Pack(want[:])) {
		t.Fatalf("Encode(%x) = %x, reference %x", frame, slot, unpacked.Pack(want[:]))
	}
	if got, res := Decode(slot); got != frame || res != (fec.Result{BitsChecked: 47}) {
		t.Fatalf("Decode(Encode(%x)) = %x %+v", frame, got, res)
	}

	var rx [9]byte
	for i := range rx {
		rx[i] = slot[i] ^ mask[i]
	}
	got, res := Decode(rx)
	var in [72]unpacked.Bit
	copy(in[:], unpacked.Unpack(rx[:]))
	if want, wantRes := refambe.Decode(in); got != [7]byte(unpacked.Pack(want[:])) || res != wantRes {
		t.Fatalf("Decode(%x) = %x %+v, reference %x %+v", rx, got, res, unpacked.Pack(want[:]), wantRes)
	}
	if got[6]&0x7F != 0 || res.BitsChecked != 47 || res.ErrorsCorrected < 0 || res.ErrorsCorrected > 6 {
		t.Fatalf("Decode(%x) = %x %+v", rx, got, res)
	}
	m0, m1, m2, m3 := columns(&mask)
	nA := bits.OnesCount32(m0<<6 | m1>>12)
	nB := bits.OnesCount32((m1&0xFFF)<<11 | m2>>7)
	mC := uint64((m2&0x7F)<<18 | m3)
	if got := frameBits(got) & 0x1FFFFFF; got != frameBits(frame)&0x1FFFFFF^mC {
		t.Fatalf("Decode(%x) unprotected bits %07x, want %07x", rx, got, frameBits(frame)&0x1FFFFFF^mC)
	}
	if nA <= 3 && nB <= 3 {
		want := frameBits(frame) ^ mC
		if frameBits(got) != want || res != (fec.Result{BitsChecked: 47, ErrorsCorrected: nA + nB}) {
			t.Fatalf("%d+%d errors %x: Decode = %x %+v, want %013x", nA, nB, mask, got, res, want)
		}
	}
	if nA == 4 && !res.Uncorrectable {
		t.Fatalf("4 errors in section A %x: Decode accepted %x %+v", mask, got, res)
	}
	if !res.Uncorrectable {
		re := Encode(got)
		d := 0
		for i := range re {
			d += bits.OnesCount8(re[i] ^ rx[i])
		}
		if d != res.ErrorsCorrected {
			t.Fatalf("Decode(%x) = %x %+v, but Encode of it is %d bits away", rx, got, res, d)
		}
	}
}

// FuzzAMBE encodes a fuzzed 49-bit frame into a 72-bit slot, adds the
// error pattern mask and decodes. Encode and Decode agree with the
// reference codec, and an error-free slot decodes to the frame. Up to 3
// errors in each Golay section decode to the frame with ErrorsCorrected
// equal to their count, and errors in the unprotected bits pass through.
// 4 errors in the Golay(24,12) section set Uncorrectable. A decode without
// Uncorrectable re-encodes to a slot exactly ErrorsCorrected bits from the
// received slot.
func FuzzAMBE(f *testing.F) {
	f.Add([]byte{0xF8, 0x01, 0xA9, 0x9F, 0x8C, 0xE0, 0x80}, []byte{})
	f.Add([]byte{0xF8, 0x01, 0xA9, 0x9F, 0x8C, 0xE0, 0x80}, []byte{0x80, 0, 0, 0, 0, 0, 0, 0, 0x01})
	f.Add([]byte{1, 2, 3, 4, 5, 6, 0x80}, []byte{0, 0x08})
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, []byte{0xF0, 0x0F, 0xF0, 0x0F, 0xF0, 0x0F, 0xF0, 0x0F, 0xF0})
	f.Add([]byte{}, []byte{0xB9, 0xE8, 0x81, 0x52, 0x61, 0x73, 0x00, 0x2A, 0x6B})
	f.Fuzz(func(t *testing.T, frame, mask []byte) {
		var fr [7]byte
		var m [9]byte
		copy(fr[:], frame)
		copy(m[:], mask)
		checkAMBE(t, fr, m)
	})
}
