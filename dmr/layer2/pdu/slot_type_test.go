package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/dmr/bit"
	"github.com/USA-RedDragon/dmrgo/dmr/fec/golay"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/pdu"
)

// encodeSlotType builds 20-bit Golay(20,8,7) coded slot type data
// from a color code (4 bits) and data type (4 bits).
func encodeSlotType(colorCode int, dataType elements.DataType) [20]bit.Bit {
	var data [8]bit.Bit
	for i := 0; i < 4; i++ {
		if (colorCode>>(3-i))&1 == 1 {
			data[i] = 1
		}
	}
	dt := byte(dataType)
	for i := 0; i < 4; i++ {
		if (dt>>(3-i))&1 == 1 {
			data[4+i] = 1
		}
	}
	parity := golay.Golay_20_8_Parity(data)
	var result [20]bit.Bit
	copy(result[:8], data[:])
	copy(result[8:], parity[:])
	return result
}

func TestSlotType_DecodeEncodeRoundTrip(t *testing.T) {
	cc := 5
	dt := elements.DataTypeVoiceLCHeader

	encoded := encodeSlotType(cc, dt)
	st := pdu.NewSlotTypeFromBits(encoded)

	if st.ColorCode != cc {
		t.Errorf("ColorCode = %d, want %d", st.ColorCode, cc)
	}
	if st.DataType != dt {
		t.Errorf("DataType = %d, want %d", st.DataType, dt)
	}
	if st.FEC.Uncorrectable {
		t.Error("FEC should not be uncorrectable for clean data")
	}
	if st.FEC.ErrorsCorrected != 0 {
		t.Errorf("ErrorsCorrected = %d, want 0 for clean data", st.FEC.ErrorsCorrected)
	}
}

func TestSlotType_AllColorCodes(t *testing.T) {
	dt := elements.DataTypeVoiceLCHeader
	for cc := 0; cc < 16; cc++ {
		encoded := encodeSlotType(cc, dt)
		st := pdu.NewSlotTypeFromBits(encoded)
		if st.ColorCode != cc {
			t.Errorf("cc=%d: got ColorCode=%d", cc, st.ColorCode)
		}
		if st.FEC.Uncorrectable {
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
		encoded := encodeSlotType(1, dt)
		st := pdu.NewSlotTypeFromBits(encoded)
		if st.DataType != dt {
			t.Errorf("dt=%d: got DataType=%d", dt, st.DataType)
		}
		if st.FEC.Uncorrectable {
			t.Errorf("dt=%d: FEC uncorrectable", dt)
		}
	}
}

func TestSlotType_ToString(t *testing.T) {
	encoded := encodeSlotType(3, elements.DataTypeCSBK)
	st := pdu.NewSlotTypeFromBits(encoded)
	s := st.ToString()
	if s == "" {
		t.Error("ToString() should not be empty")
	}
}

func TestSlotType_SingleBitError(t *testing.T) {
	encoded := encodeSlotType(7, elements.DataTypeVoiceLCHeader)
	// Flip one bit - Golay should correct it
	encoded[10] ^= 1
	st := pdu.NewSlotTypeFromBits(encoded)
	if st.ColorCode != 7 {
		t.Errorf("ColorCode = %d, want 7 after single-bit error correction", st.ColorCode)
	}
	if st.FEC.Uncorrectable {
		t.Error("single-bit error should be correctable")
	}
}
