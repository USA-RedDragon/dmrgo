package pdu_test

import (
	"encoding/hex"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec/reedsolomon"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// realVoiceLCHeader is an on-air Voice LC Header codeword (UU_V_Ch_Usr,
// target 9990, source 3191868). Its RS(12,9) parity is cb5957 once the
// 0x969696 Data Type CRC Mask is removed.
const realVoiceLCHeader = "03000000270630b43c5dcfc1"

func hexBits96(t *testing.T, s string) [96]bit.Bit {
	t.Helper()
	raw, err := hex.DecodeString(s)
	if err != nil || len(raw) != 12 {
		t.Fatalf("bad hex %q", s)
	}
	var bits [96]bit.Bit
	copy(bits[:], bit.UnpackBits(raw))
	return bits
}

func packedFLC(t *testing.T, flc *pdu.FullLinkControl) []byte {
	t.Helper()
	bits := must(pdu.EncodeFullLinkControl(flc))
	return bit.PackBits(bits[:])
}

// ETSI TS 102 361-1 B.3.12, Table B.21: the mask is XORed into the parity
// only, and only for Voice LC Header and Terminator with LC.
func TestFLC_DataTypeMaskAppliedToParity(t *testing.T) {
	base := pdu.FullLinkControl{
		FLCO:         enums.FLCOGroupVoiceChannelUser,
		FeatureSetID: enums.StandardizedFID,
		GroupVoice:   &pdu.FLCGroupVoice{GroupAddress: 9990, SourceAddress: 3120101},
	}

	tests := []struct {
		dataType enums.DataType
		mask     [3]byte
	}{
		{enums.DataTypeVoiceLCHeader, [3]byte{0x96, 0x96, 0x96}},
		{enums.DataTypeTerminatorWithLC, [3]byte{0x99, 0x99, 0x99}},
	}
	for _, tt := range tests {
		t.Run(tt.dataType.String(), func(t *testing.T) {
			flc := base
			flc.DataType = tt.dataType
			masked := packedFLC(t, &flc)
			unmasked := reedsolomon.Encode([9]byte(masked[:9]))
			for i := 0; i < 3; i++ {
				if got := masked[9+i] ^ unmasked[9+i]; got != tt.mask[i] {
					t.Errorf("parity byte %d XOR = %#x, want %#x", 9+i, got, tt.mask[i])
				}
			}
		})
	}
}

// RS-protected Full LC is sent only in a Voice LC Header or a Terminator
// with LC, so encoding with any other data type is an error.
func TestFLC_EncodeRejectsUnmaskedDataType(t *testing.T) {
	for _, dt := range []enums.DataType{enums.DataTypePIHeader, enums.DataTypeCSBK, enums.DataTypeIdle} {
		flc := pdu.FullLinkControl{
			DataType:     dt,
			FLCO:         enums.FLCOGroupVoiceChannelUser,
			FeatureSetID: enums.StandardizedFID,
			GroupVoice:   &pdu.FLCGroupVoice{GroupAddress: 9990, SourceAddress: 3120101},
		}
		if _, err := pdu.EncodeFullLinkControl(&flc); err == nil {
			t.Errorf("%s: EncodeFullLinkControl returned no error", dt.String())
		}
	}
}

func TestFLC_DecodeRealVoiceLCHeader(t *testing.T) {
	flc, fecResult := pdu.DecodeFullLinkControl(hexBits96(t, realVoiceLCHeader), enums.DataTypeVoiceLCHeader)
	if fecResult.Uncorrectable || fecResult.ErrorsCorrected != 0 {
		t.Fatalf("FEC = %+v, want a clean codeword", fecResult)
	}
	if flc.DataType != enums.DataTypeVoiceLCHeader {
		t.Errorf("DataType = %v, want Voice LC Header", flc.DataType)
	}
	if flc.FLCO != enums.FLCOUnitToUnitVoiceChannelUser || flc.UnitToUnit == nil {
		t.Fatalf("FLCO = %v, want UU_V_Ch_Usr", flc.FLCO)
	}
	if flc.UnitToUnit.TargetAddress != 9990 || flc.UnitToUnit.SourceAddress != 3191868 {
		t.Errorf("target/source = %d/%d, want 9990/3191868", flc.UnitToUnit.TargetAddress, flc.UnitToUnit.SourceAddress)
	}
}

// Depends on a correct RS(12,9) decoder: a wrong mask leaves three bad
// parity bytes, which RS(12,9) cannot correct.
func TestFLC_DecodeRealVoiceLCHeaderWithWrongMaskFails(t *testing.T) {
	for _, dt := range []enums.DataType{enums.DataTypeTerminatorWithLC, enums.DataTypePIHeader} {
		_, fecResult := pdu.DecodeFullLinkControl(hexBits96(t, realVoiceLCHeader), dt)
		if !fecResult.Uncorrectable && fecResult.ErrorsCorrected == 0 {
			t.Errorf("%s: FEC = %+v, want an RS failure", dt.String(), fecResult)
		}
	}
}

// Depends on a correct RS(12,9) encoder.
func TestFLC_EncodeRealVoiceLCHeader(t *testing.T) {
	want := hexBits96(t, realVoiceLCHeader)
	flc, _ := pdu.DecodeFullLinkControl(want, enums.DataTypeVoiceLCHeader)
	if got := must(pdu.EncodeFullLinkControl(&flc)); got != want {
		t.Errorf("EncodeFullLinkControl = %x, want %s", bit.PackBits(got[:]), realVoiceLCHeader)
	}
}
