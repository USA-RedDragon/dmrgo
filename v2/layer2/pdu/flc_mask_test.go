package pdu_test

import (
	"encoding/hex"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/reedsolomon"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// realVoiceLCHeader is an on-air Voice LC Header codeword (UU_V_Ch_Usr,
// target 9990, source 3191868). Its RS(12,9) parity is cb5957 once the
// 0x969696 Data Type CRC Mask is removed.
const realVoiceLCHeader = "03000000270630b43c5dcfc1"

func hex12(t *testing.T, s string) [12]byte {
	t.Helper()
	raw, err := hex.DecodeString(s)
	if err != nil || len(raw) != 12 {
		t.Fatalf("bad hex %q", s)
	}
	return [12]byte(raw)
}

// ETSI TS 102 361-1 B.3.12, Table B.21: the mask is XORed into the parity
// only, and only for Voice LC Header and Terminator with LC.
func TestFLC_DataTypeMaskAppliedToParity(t *testing.T) {
	t.Parallel()

	base := pdu.FullLinkControl{
		FLCO:       enums.FLCOGroupVoiceChannelUser,
		FID:        enums.FIDStandardized,
		GroupVoice: &pdu.FLCGroupVoice{GroupAddress: 9990, SourceAddress: 3120101},
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
			t.Parallel()

			flc := base
			flc.BurstDataType = tt.dataType
			masked := must(flc.Encode())
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
	t.Parallel()

	for _, dt := range []enums.DataType{enums.DataTypePIHeader, enums.DataTypeCSBK, enums.DataTypeIdle} {
		flc := pdu.FullLinkControl{
			BurstDataType: dt,
			FLCO:          enums.FLCOGroupVoiceChannelUser,
			FID:           enums.FIDStandardized,
			GroupVoice:    &pdu.FLCGroupVoice{GroupAddress: 9990, SourceAddress: 3120101},
		}
		if _, err := flc.Encode(); err == nil {
			t.Errorf("%s: FullLinkControl.Encode returned no error", dt.String())
		}
	}
}

func TestFLC_DecodeRealVoiceLCHeader(t *testing.T) {
	t.Parallel()

	flc := pdu.DecodeFullLinkControl(hex12(t, realVoiceLCHeader), enums.DataTypeVoiceLCHeader)
	fecResult := flc.FEC
	if fecResult.Uncorrectable || fecResult.ErrorsCorrected != 0 {
		t.Fatalf("FEC = %+v, want a clean codeword", fecResult)
	}
	if flc.DataType() != enums.DataTypeVoiceLCHeader {
		t.Errorf("DataType = %v, want Voice LC Header", flc.DataType())
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
	t.Parallel()

	for _, dt := range []enums.DataType{enums.DataTypeTerminatorWithLC, enums.DataTypePIHeader} {
		fecResult := pdu.DecodeFullLinkControl(hex12(t, realVoiceLCHeader), dt).FEC
		if !fecResult.Uncorrectable && fecResult.ErrorsCorrected == 0 {
			t.Errorf("%s: FEC = %+v, want an RS failure", dt.String(), fecResult)
		}
	}
}

// Depends on a correct RS(12,9) encoder.
func TestFLC_EncodeRealVoiceLCHeader(t *testing.T) {
	t.Parallel()

	want := hex12(t, realVoiceLCHeader)
	flc := pdu.DecodeFullLinkControl(want, enums.DataTypeVoiceLCHeader)
	if got := must(flc.Encode()); got != want {
		t.Errorf("FullLinkControl.Encode = %x, want %s", got, realVoiceLCHeader)
	}
}
