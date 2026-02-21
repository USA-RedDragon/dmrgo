package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/pdu"
)

func TestSlotType_DecodeEncodeRoundTrip(t *testing.T) {
	cc := 5
	dt := elements.DataTypeVoiceLCHeader

	encoded := pdu.EncodeSlotType(&pdu.SlotType{ColorCode: cc, DataType: dt})
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
	dt := elements.DataTypeVoiceLCHeader
	for cc := 0; cc < 16; cc++ {
		encoded := pdu.EncodeSlotType(&pdu.SlotType{ColorCode: cc, DataType: dt})
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
	dataTypes := []elements.DataType{
		elements.DataTypePIHeader,
		elements.DataTypeVoiceLCHeader,
		elements.DataTypeTerminatorWithLC,
		elements.DataTypeCSBK,
		elements.DataTypeMBCHeader,
		elements.DataTypeMBCContinuation,
		elements.DataTypeDataHeader,
		elements.DataTypeRate12,
		elements.DataTypeRate34,
		elements.DataTypeIdle,
		elements.DataTypeRate1,
		elements.DataTypeUnifiedSingleBlock,
	}

	for _, dt := range dataTypes {
		encoded := pdu.EncodeSlotType(&pdu.SlotType{ColorCode: 1, DataType: dt})
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
	encoded := pdu.EncodeSlotType(&pdu.SlotType{ColorCode: 3, DataType: elements.DataTypeCSBK})
	st, _ := pdu.DecodeSlotType(encoded)
	s := st.ToString()
	if s == "" {
		t.Error("ToString() should not be empty")
	}
}

func TestSlotType_SingleBitError(t *testing.T) {
	encoded := pdu.EncodeSlotType(&pdu.SlotType{ColorCode: 7, DataType: elements.DataTypeVoiceLCHeader})
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
		DataType:  elements.DataTypeCSBK,
	}
	encoded := pdu.EncodeSlotType(&st)
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
