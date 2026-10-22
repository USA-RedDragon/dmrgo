package pdu_test

import (
	"encoding/hex"
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// ETSI TS 102 361-1 Table B.18
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
	t.Parallel()

	tests := []struct {
		dt    enums.DataType
		mask  uint32
		width int
		ok    bool
	}{
		{enums.DataTypePIHeader, 0x6969, 16, true},
		{enums.DataTypeVoiceLCHeader, 0x969696, 24, true},
		{enums.DataTypeTerminatorWithLC, 0x999999, 24, true},
		{enums.DataTypeCSBK, 0xA5A5, 16, true},
		{enums.DataTypeMBCHeader, 0xAAAA, 16, true},
		{enums.DataTypeMBCContinuation, 0, 0, false},
		{enums.DataTypeDataHeader, 0xCCCC, 16, true},
		{enums.DataTypeRate12, 0x0F0, 9, true},
		{enums.DataTypeRate34, 0x1FF, 9, true},
		{enums.DataTypeIdle, 0, 0, false},
		{enums.DataTypeRate1, 0x10F, 9, true},
		{enums.DataTypeUnifiedSingleBlock, 0x3333, 16, true},
		{enums.DataTypeReserved, 0, 0, false},
		{enums.DataType(15), 0, 0, false},
	}
	for _, tt := range tests {
		mask, width, ok := pdu.CRCMask(tt.dt)
		if mask != tt.mask || width != tt.width || ok != tt.ok {
			t.Errorf("CRCMask(%d) = %#x, %d, %t; want %#x, %d, %t", tt.dt, mask, width, ok, tt.mask, tt.width, tt.ok)
		}
	}
}

func TestLCBytesCodeword_MatchesTableB18(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(1))
	for n := 0; n < 1000; n++ {
		var lc [9]byte
		rng.Read(lc[:])
		p := matrixParity(lc)
		for _, dt := range []enums.DataType{enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC} {
			mask, _, _ := pdu.CRCMask(dt)
			lcb := pdu.LCBytes(lc)
			cw, err := lcb.Codeword(dt)
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

func TestLCBytesCodeword_RealCapture(t *testing.T) {
	t.Parallel()

	lc := hex9(t, "03000000270630b43c")
	tests := []struct {
		dt   enums.DataType
		want string
	}{
		{enums.DataTypeVoiceLCHeader, "03000000270630b43c5dcfc1"},
		{enums.DataTypeTerminatorWithLC, "03000000270630b43c52c0ce"},
	}
	for _, tt := range tests {
		lcb := pdu.LCBytes(lc)
		cw, err := lcb.Codeword(tt.dt)
		if err != nil {
			t.Fatal(err)
		}
		if got := hex.EncodeToString(cw[:]); got != tt.want {
			t.Errorf("dt %d: got %s, want %s", tt.dt, got, tt.want)
		}
		flc := pdu.DecodeFullLinkControl(hex12(t, tt.want), tt.dt)
		res := flc.FEC
		if res.HasErrors() {
			t.Errorf("dt %d: generated decoder reports %+v", tt.dt, res)
		}
		enc := must(flc.Encode())
		if hex.EncodeToString(enc[:]) != tt.want {
			t.Errorf("dt %d: generated encoder disagrees", tt.dt)
		}
		other := enums.DataTypeVoiceLCHeader
		if tt.dt == other {
			other = enums.DataTypeTerminatorWithLC
		}
		if pdu.FullLCParityOK(cw, other) {
			t.Errorf("dt %d: codeword passes with the other mask", tt.dt)
		}
	}
}

func TestFullLinkControlEncode(t *testing.T) {
	t.Parallel()

	lc := hex9(t, "03000000270630b43c")
	f := pdu.DecodeFullLinkControlBytes(lc)
	for dt, want := range map[enums.DataType]string{
		enums.DataTypeVoiceLCHeader:    "03000000270630b43c5dcfc1",
		enums.DataTypeTerminatorWithLC: "03000000270630b43c52c0ce",
	} {
		f.BurstDataType = dt
		cw := must(f.Encode())
		if got := hex.EncodeToString(cw[:]); got != want {
			t.Errorf("dt %s: FullLinkControl.Encode %s, want %s", dt, got, want)
		}
		lcb := pdu.LCBytes(lc)
		p := must(lcb.Codeword(dt))
		p[9], p[10], p[11] = 0, 0, 0
		if err := pdu.Seal(&p, dt); err != nil || hex.EncodeToString(p[:]) != want {
			t.Errorf("dt %s: Seal gives %x %v, want %s", dt, p, err, want)
		}
	}
	f.BurstDataType = enums.DataTypeCSBK
	if _, err := f.Encode(); err == nil {
		t.Error("FullLinkControl.Encode accepts a CSBK data type")
	}
	f.BurstDataType = enums.DataTypeVoiceLCHeader
	f.FLCO = 64
	if _, err := f.Encode(); err == nil {
		t.Error("FullLinkControl.Encode accepts FLCO 64")
	}
}

func TestLCBytesCodeword_RejectsOtherDataTypes(t *testing.T) {
	t.Parallel()

	for dt := enums.DataType(0); dt <= 15; dt++ {
		if dt == enums.DataTypeVoiceLCHeader || dt == enums.DataTypeTerminatorWithLC {
			continue
		}
		lcb := pdu.LCBytes{}
		if _, err := lcb.Codeword(dt); err == nil {
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
	t.Parallel()

	lc := hex9(t, "03000000270630b43c")
	lcb := pdu.LCBytes(lc)
	cw := must(lcb.Codeword(enums.DataTypeVoiceLCHeader))
	got, res, err := pdu.DecodeFullLCCodeword(cw, enums.DataTypeVoiceLCHeader)
	if err != nil || got != lc || res.HasErrors() {
		t.Fatalf("clean: %x %+v %v", got, res, err)
	}
	cw[4] ^= 0x5A
	if pdu.FullLCParityOK(cw, enums.DataTypeVoiceLCHeader) {
		t.Fatal("strict check accepts a corrupted codeword")
	}
	got, res, err = pdu.DecodeFullLCCodeword(cw, enums.DataTypeVoiceLCHeader)
	if err != nil || got != lc || res.ErrorsCorrected == 0 || res.Uncorrectable {
		t.Fatalf("corrupted: %x %+v %v", got, res, err)
	}
}

func TestSeal_MatchesGeneratedEncoders(t *testing.T) {
	t.Parallel()

	csbk := must((&pdu.CSBK{}).Encode())
	mbc := must((&pdu.MBCHeader{}).Encode())
	pi := must((&pdu.PIHeader{AlgorithmID: 1, KeyID: 7, MessageIndicator: 0xDEADBEEF, Destination: 9990}).Encode())
	usbd := must((&pdu.UnifiedSingleBlockData{}).Encode())
	tests := []struct {
		dt   enums.DataType
		bits [12]byte
	}{
		{enums.DataTypeCSBK, csbk},
		{enums.DataTypeMBCHeader, mbc},
		{enums.DataTypePIHeader, pi},
		{enums.DataTypeUnifiedSingleBlock, usbd},
	}
	for _, tt := range tests {
		b := tt.bits
		if !pdu.BlockCRCOK(b, tt.dt) {
			t.Errorf("dt %d: BlockCRCOK rejects generated block %x", tt.dt, b)
		}
		sealed := b
		sealed[10], sealed[11] = 0, 0
		if err := pdu.Seal(&sealed, tt.dt); err != nil || sealed != b {
			t.Errorf("dt %d: Seal gives %x (%v), want %x", tt.dt, sealed, err, b)
		}
	}
}

func TestSeal_RoundTripAndRejects(t *testing.T) {
	t.Parallel()

	singles := []enums.DataType{
		enums.DataTypePIHeader, enums.DataTypeCSBK, enums.DataTypeMBCHeader,
		enums.DataTypeDataHeader, enums.DataTypeUnifiedSingleBlock,
	}
	rng := rand.New(rand.NewSource(2))
	for _, dt := range singles {
		for n := 0; n < 100; n++ {
			var b [12]byte
			rng.Read(b[:])
			if err := pdu.Seal(&b, dt); err != nil {
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
	for _, dt := range []enums.DataType{
		enums.DataTypeMBCContinuation, enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeRate1,
		enums.DataTypeIdle, enums.DataTypeReserved,
	} {
		b := [12]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
		before := b
		if err := pdu.Seal(&b, dt); err == nil || b != before {
			t.Errorf("dt %d: Seal accepted or changed the block", dt)
		}
		if pdu.BlockCRCOK(b, dt) {
			t.Errorf("dt %d: BlockCRCOK true", dt)
		}
	}
	for _, dt := range []enums.DataType{enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC} {
		b := [12]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
		if err := pdu.Seal(&b, dt); err != nil || !pdu.FullLCParityOK(b, dt) || pdu.BlockCRCOK(b, dt) {
			t.Errorf("dt %d: Seal gives %x %v", dt, b, err)
		}
	}
}
