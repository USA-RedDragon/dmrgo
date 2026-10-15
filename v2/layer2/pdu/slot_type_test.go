package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestSlotType_DecodeEncodeRoundTrip(t *testing.T) {
	cc := 5
	dt := enums.DataTypeVoiceLCHeader

	encoded := must(pdu.EncodeSlotType(&pdu.SlotType{ColorCode: cc, DataType: dt}))
	st, fecResult := pdu.DecodeSlotType(encoded)

	if st.ColorCode != cc {
		t.Errorf("ColorCode = %d, want %d", st.ColorCode, cc)
	}
	if st.DataType != dt {
		t.Errorf("DataType = %d, want %d", st.DataType, dt)
	}
	if fecResult.Uncorrectable {
		t.Error("FEC should not be uncorrectable for clean data")
	}
	if fecResult.ErrorsCorrected != 0 {
		t.Errorf("ErrorsCorrected = %d, want 0 for clean data", fecResult.ErrorsCorrected)
	}
}

func TestSlotType_AllColorCodes(t *testing.T) {
	dt := enums.DataTypeVoiceLCHeader
	for cc := 0; cc < 16; cc++ {
		encoded := must(pdu.EncodeSlotType(&pdu.SlotType{ColorCode: cc, DataType: dt}))
		st, fecResult := pdu.DecodeSlotType(encoded)
		if st.ColorCode != cc {
			t.Errorf("cc=%d: got ColorCode=%d", cc, st.ColorCode)
		}
		if fecResult.Uncorrectable {
			t.Errorf("cc=%d: FEC uncorrectable", cc)
		}
	}
}

func TestSlotType_AllDataTypes(t *testing.T) {
	dataTypes := []enums.DataType{
		enums.DataTypePIHeader,
		enums.DataTypeVoiceLCHeader,
		enums.DataTypeTerminatorWithLC,
		enums.DataTypeCSBK,
		enums.DataTypeMBCHeader,
		enums.DataTypeMBCContinuation,
		enums.DataTypeDataHeader,
		enums.DataTypeRate12,
		enums.DataTypeRate34,
		enums.DataTypeIdle,
		enums.DataTypeRate1,
		enums.DataTypeUnifiedSingleBlock,
	}

	for _, dt := range dataTypes {
		encoded := must(pdu.EncodeSlotType(&pdu.SlotType{ColorCode: 1, DataType: dt}))
		st, fecResult := pdu.DecodeSlotType(encoded)
		if st.DataType != dt {
			t.Errorf("dt=%d: got DataType=%d", dt, st.DataType)
		}
		if fecResult.Uncorrectable {
			t.Errorf("dt=%d: FEC uncorrectable", dt)
		}
	}
}

func TestSlotType_ToString(t *testing.T) {
	encoded := must(pdu.EncodeSlotType(&pdu.SlotType{ColorCode: 3, DataType: enums.DataTypeCSBK}))
	st, _ := pdu.DecodeSlotType(encoded)
	s := st.ToString()
	if s == "" {
		t.Error("ToString() should not be empty")
	}
}

func TestSlotType_SingleBitError(t *testing.T) {
	encoded := must(pdu.EncodeSlotType(&pdu.SlotType{ColorCode: 7, DataType: enums.DataTypeVoiceLCHeader}))
	// Flip one bit - Golay should correct it
	encoded[10] ^= 1
	st, fecResult := pdu.DecodeSlotType(encoded)
	if st.ColorCode != 7 {
		t.Errorf("ColorCode = %d, want 7 after single-bit error correction", st.ColorCode)
	}
	if fecResult.Uncorrectable {
		t.Error("single-bit error should be correctable")
	}
}

func TestEncodeSlotType_RoundTrip(t *testing.T) {
	st := pdu.SlotType{
		ColorCode: 5,
		DataType:  enums.DataTypeCSBK,
	}
	encoded := must(pdu.EncodeSlotType(&st))
	// Full round-trip: encode produces [20]bit.Bit with Golay parity
	decoded, fecResult := pdu.DecodeSlotType(encoded)
	if fecResult.Uncorrectable {
		t.Error("FEC should not be uncorrectable for clean encoded data")
	}
	if decoded.ColorCode != st.ColorCode {
		t.Errorf("round-trip ColorCode = %d, want %d", decoded.ColorCode, st.ColorCode)
	}
	if decoded.DataType != st.DataType {
		t.Errorf("round-trip DataType = %d, want %d", decoded.DataType, st.DataType)
	}
}
