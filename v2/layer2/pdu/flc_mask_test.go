package pdu_test

import (
	"encoding/hex"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/reedsolomon"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const realVoiceLCHeader = "03000000270630b43c5dcfc1"

func hex12(t *testing.T, s string) [12]byte {
	t.Helper()
	raw, err := hex.DecodeString(s)
	if err != nil || len(raw) != 12 {
		t.Fatalf("bad hex %q", s)
	}
	return [12]byte(raw)
}

// ETSI TS 102 361-1; Table B.21
func TestFLC_DataTypeMaskAppliedToParity(t *testing.T) {
	t.Parallel()

	var base pdu.FullLinkControl
	noErr(base.SetGroupVoice(pdu.FLCGroupVoice{GroupAddress: 9990, SourceAddress: 3120101}))

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

func TestFLC_EncodeRejectsUnmaskedDataType(t *testing.T) {
	t.Parallel()

	for _, dt := range []enums.DataType{enums.DataTypePIHeader, enums.DataTypeCSBK, enums.DataTypeIdle} {
		flc := pdu.FullLinkControl{BurstDataType: dt}
		noErr(flc.SetGroupVoice(pdu.FLCGroupVoice{GroupAddress: 9990, SourceAddress: 3120101}))
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
	uu, ok := flc.AsUnitToUnit()
	if flc.FLCO != enums.FLCOUnitToUnitVoiceChannelUser || !ok {
		t.Fatalf("FLCO = %v, want UU_V_Ch_Usr", flc.FLCO)
	}
	if uu.TargetAddress != 9990 || uu.SourceAddress != 3191868 {
		t.Errorf("target/source = %d/%d, want 9990/3191868", uu.TargetAddress, uu.SourceAddress)
	}
}

func TestFLC_DecodeRealVoiceLCHeaderWithWrongMaskFails(t *testing.T) {
	t.Parallel()

	for _, dt := range []enums.DataType{enums.DataTypeTerminatorWithLC, enums.DataTypePIHeader} {
		fecResult := pdu.DecodeFullLinkControl(hex12(t, realVoiceLCHeader), dt).FEC
		if !fecResult.Uncorrectable && fecResult.ErrorsCorrected == 0 {
			t.Errorf("%s: FEC = %+v, want an RS failure", dt.String(), fecResult)
		}
	}
}

func TestFLC_EncodeRealVoiceLCHeader(t *testing.T) {
	t.Parallel()

	want := hex12(t, realVoiceLCHeader)
	flc := pdu.DecodeFullLinkControl(want, enums.DataTypeVoiceLCHeader)
	if got := must(flc.Encode()); got != want {
		t.Errorf("FullLinkControl.Encode = %x, want %s", got, realVoiceLCHeader)
	}
}
