package pdu_test

import (
	"encoding/hex"
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// matrixParity computes the RS(12,9) parity with the parity columns of the
// ETSI TS 102 361-1 Table B.18 generator matrix, one row per LC octet.
func matrixParity(lc [9]byte) [3]byte {
	rsGenerator := [9][3]byte{
		{0x1C, 0xBC, 0xFD},
		{0x89, 0x31, 0x08},
		{0xAD, 0x41, 0x36},
		{0x7D, 0x71, 0x16},
		{0xF3, 0xA6, 0x3A},
		{0x08, 0x83, 0x7B},
		{0x3F, 0x6F, 0x02},
		{0x6C, 0x0D, 0xA7},
		{0x0E, 0x38, 0x40},
	}
	var p [3]byte
	for i, m := range lc {
		for j := range p {
			p[j] ^= gfMul(m, rsGenerator[i][j])
		}
	}
	return p
}

func gfMul(a, b byte) byte {
	var p byte
	for b != 0 {
		if b&1 != 0 {
			p ^= a
		}
		hi := a & 0x80
		a <<= 1
		if hi != 0 {
			a ^= 0x1D
		}
		b >>= 1
	}
	return p
}

func hex9(t *testing.T, s string) [9]byte {
	t.Helper()
	var out [9]byte
	raw, err := hex.DecodeString(s)
	if err != nil || len(raw) != 9 {
		t.Fatalf("bad hex %q", s)
	}
	copy(out[:], raw)
	return out
}

func TestCRCMask_TableB21(t *testing.T) {
	tests := []struct {
		dt    elements.DataType
		mask  uint32
		width int
		ok    bool
	}{
		{elements.DataTypePIHeader, 0x6969, 16, true},
		{elements.DataTypeVoiceLCHeader, 0x969696, 24, true},
		{elements.DataTypeTerminatorWithLC, 0x999999, 24, true},
		{elements.DataTypeCSBK, 0xA5A5, 16, true},
		{elements.DataTypeMBCHeader, 0xAAAA, 16, true},
		{elements.DataTypeMBCContinuation, 0, 0, false},
		{elements.DataTypeDataHeader, 0xCCCC, 16, true},
		{elements.DataTypeRate12, uint32(pdu.CRC9MaskRate12), 9, true},
		{elements.DataTypeRate34, uint32(pdu.CRC9MaskRate34), 9, true},
		{elements.DataTypeIdle, 0, 0, false},
		{elements.DataTypeRate1, uint32(pdu.CRC9MaskRate1), 9, true},
		{elements.DataTypeUnifiedSingleBlock, 0x3333, 16, true},
		{elements.DataTypeReserved, 0, 0, false},
		{elements.DataType(15), 0, 0, false},
	}
	for _, tt := range tests {
		mask, width, ok := pdu.CRCMask(tt.dt)
		if mask != tt.mask || width != tt.width || ok != tt.ok {
			t.Errorf("CRCMask(%d) = %#x, %d, %t; want %#x, %d, %t", tt.dt, mask, width, ok, tt.mask, tt.width, tt.ok)
		}
	}
}

func TestFullLCCodeword_MatchesTableB18(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for n := 0; n < 1000; n++ {
		var lc [9]byte
		rng.Read(lc[:])
		p := matrixParity(lc)
		for _, dt := range []elements.DataType{elements.DataTypeVoiceLCHeader, elements.DataTypeTerminatorWithLC} {
			mask, _, _ := pdu.CRCMask(dt)
			cw, err := pdu.FullLCCodeword(lc, dt)
			if err != nil {
				t.Fatal(err)
			}
			if [9]byte(cw[:9]) != lc {
				t.Fatalf("LC octets changed: %x", cw)
			}
			want := [3]byte{p[0] ^ byte(mask>>16), p[1] ^ byte(mask>>8), p[2] ^ byte(mask)}
			if [3]byte(cw[9:]) != want {
				t.Fatalf("lc %x dt %d: parity %x, want %x", lc, dt, cw[9:], want)
			}
			if !pdu.FullLCParityOK(cw, dt) {
				t.Fatalf("FullLCParityOK rejects %x", cw)
			}
		}
	}
}

func TestFullLCCodeword_RealCapture(t *testing.T) {
	lc := hex9(t, "03000000270630b43c")
	tests := []struct {
		dt   elements.DataType
		want string
	}{
		{elements.DataTypeVoiceLCHeader, "03000000270630b43c5dcfc1"},
		{elements.DataTypeTerminatorWithLC, "03000000270630b43c52c0ce"},
	}
	for _, tt := range tests {
		cw, err := pdu.FullLCCodeword(lc, tt.dt)
		if err != nil {
			t.Fatal(err)
		}
		if got := hex.EncodeToString(cw[:]); got != tt.want {
			t.Errorf("dt %d: got %s, want %s", tt.dt, got, tt.want)
		}
		bits := hexBits96(t, tt.want)
		flc, res := pdu.DecodeFullLinkControl(bits, tt.dt)
		if res.HasErrors() {
			t.Errorf("dt %d: generated decoder reports %+v", tt.dt, res)
		}
		enc := must(pdu.EncodeFullLinkControl(&flc))
		if hex.EncodeToString(bit.PackBits(enc[:])) != tt.want {
			t.Errorf("dt %d: generated encoder disagrees", tt.dt)
		}
		other := elements.DataTypeVoiceLCHeader
		if tt.dt == other {
			other = elements.DataTypeTerminatorWithLC
		}
		if pdu.FullLCParityOK(cw, other) {
			t.Errorf("dt %d: codeword passes with the other mask", tt.dt)
		}
	}
}

func TestFullLCCodeword_RejectsOtherDataTypes(t *testing.T) {
	for dt := elements.DataType(0); dt <= 15; dt++ {
		if dt == elements.DataTypeVoiceLCHeader || dt == elements.DataTypeTerminatorWithLC {
			continue
		}
		if _, err := pdu.FullLCCodeword([9]byte{}, dt); err == nil {
			t.Errorf("dt %d: no error", dt)
		}
		if _, _, err := pdu.DecodeFullLCCodeword([12]byte{}, dt); err == nil {
			t.Errorf("dt %d: decode: no error", dt)
		}
		if pdu.FullLCParityOK([12]byte{}, dt) {
			t.Errorf("dt %d: parity ok", dt)
		}
	}
}

func TestDecodeFullLCCodeword_CorrectsAndStrictRejects(t *testing.T) {
	lc := hex9(t, "03000000270630b43c")
	cw := must(pdu.FullLCCodeword(lc, elements.DataTypeVoiceLCHeader))
	got, res, err := pdu.DecodeFullLCCodeword(cw, elements.DataTypeVoiceLCHeader)
	if err != nil || got != lc || res.HasErrors() {
		t.Fatalf("clean: %x %+v %v", got, res, err)
	}
	cw[4] ^= 0x5A
	if pdu.FullLCParityOK(cw, elements.DataTypeVoiceLCHeader) {
		t.Fatal("strict check accepts a corrupted codeword")
	}
	got, res, err = pdu.DecodeFullLCCodeword(cw, elements.DataTypeVoiceLCHeader)
	if err != nil || got != lc || res.ErrorsCorrected == 0 || res.Uncorrectable {
		t.Fatalf("corrupted: %x %+v %v", got, res, err)
	}
}

func TestSealBlock_MatchesGeneratedEncoders(t *testing.T) {
	csbk := must(pdu.EncodeCSBK(&pdu.CSBK{}))
	mbc := must(pdu.EncodeMBCHeader(&pdu.MBCHeader{}))
	pi := must(pdu.EncodePIHeader(&pdu.PIHeader{AlgorithmID: 1, KeyID: 7, MessageIndicator: 0xDEADBEEF, Destination: 9990}))
	usbd := must(pdu.EncodeUnifiedSingleBlockData(&pdu.UnifiedSingleBlockData{}))
	tests := []struct {
		dt   elements.DataType
		bits [96]bit.Bit
	}{
		{elements.DataTypeCSBK, csbk},
		{elements.DataTypeMBCHeader, mbc},
		{elements.DataTypePIHeader, pi},
		{elements.DataTypeUnifiedSingleBlock, usbd},
	}
	for _, tt := range tests {
		var b [12]byte
		copy(b[:], bit.PackBits(tt.bits[:]))
		if !pdu.BlockCRCOK(b, tt.dt) {
			t.Errorf("dt %d: BlockCRCOK rejects generated block %x", tt.dt, b)
		}
		sealed := b
		sealed[10], sealed[11] = 0, 0
		if err := pdu.SealBlock(&sealed, tt.dt); err != nil || sealed != b {
			t.Errorf("dt %d: SealBlock gives %x (%v), want %x", tt.dt, sealed, err, b)
		}
	}
}

func TestSealBlock_RoundTripAndRejects(t *testing.T) {
	singles := []elements.DataType{
		elements.DataTypePIHeader, elements.DataTypeCSBK, elements.DataTypeMBCHeader,
		elements.DataTypeDataHeader, elements.DataTypeUnifiedSingleBlock,
	}
	rng := rand.New(rand.NewSource(2))
	for _, dt := range singles {
		for n := 0; n < 100; n++ {
			var b [12]byte
			rng.Read(b[:])
			if err := pdu.SealBlock(&b, dt); err != nil {
				t.Fatal(err)
			}
			if !pdu.BlockCRCOK(b, dt) {
				t.Fatalf("dt %d: sealed block fails", dt)
			}
			for _, other := range singles {
				if other != dt && pdu.BlockCRCOK(b, other) {
					t.Fatalf("dt %d block passes as dt %d", dt, other)
				}
			}
			b[3] ^= 0x10
			if pdu.BlockCRCOK(b, dt) {
				t.Fatalf("dt %d: corrupted block passes", dt)
			}
		}
	}
	for _, dt := range []elements.DataType{
		elements.DataTypeVoiceLCHeader, elements.DataTypeTerminatorWithLC, elements.DataTypeMBCContinuation,
		elements.DataTypeRate12, elements.DataTypeRate34, elements.DataTypeRate1, elements.DataTypeIdle,
		elements.DataTypeReserved,
	} {
		b := [12]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
		before := b
		if err := pdu.SealBlock(&b, dt); err == nil || b != before {
			t.Errorf("dt %d: SealBlock accepted or changed the block", dt)
		}
		if pdu.BlockCRCOK(b, dt) {
			t.Errorf("dt %d: BlockCRCOK true", dt)
		}
	}
}
