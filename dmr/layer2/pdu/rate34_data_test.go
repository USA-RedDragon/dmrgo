package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/dmr/bit"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/pdu"
)

func TestRate34Data_DecodeFromBits(t *testing.T) {
	// Build 96 info bits representing known packed data
	var infoBits [96]bit.Bit

	// Set byte 0 = 0xAB: 10101011
	infoBits[0] = 1
	infoBits[2] = 1
	infoBits[4] = 1
	infoBits[6] = 1
	infoBits[7] = 1

	// Set byte 1 = 0xCD: 11001101
	infoBits[8] = 1
	infoBits[9] = 1
	infoBits[12] = 1
	infoBits[13] = 1
	infoBits[15] = 1

	rt, _ := pdu.DecodeRate34Data(infoBits)
	if rt.Data[0] != 0xAB {
		t.Errorf("Data[0] = 0x%02X, want 0xAB", rt.Data[0])
	}
	if rt.Data[1] != 0xCD {
		t.Errorf("Data[1] = 0x%02X, want 0xCD", rt.Data[1])
	}
	// Remaining bytes should be zero
	for i := 2; i < 12; i++ {
		if rt.Data[i] != 0 {
			t.Errorf("Data[%d] = 0x%02X, want 0x00", i, rt.Data[i])
		}
	}
}

func TestRate34Data_GetDataType(t *testing.T) {
	rt, _ := pdu.DecodeRate34Data([96]bit.Bit{})
	rt.DataType = elements.DataTypeRate34
	if rt.GetDataType() != elements.DataTypeRate34 {
		t.Errorf("GetDataType() = %d, want DataTypeRate34", rt.GetDataType())
	}
}

func TestRate34Data_ToString(t *testing.T) {
	rt, _ := pdu.DecodeRate34Data([96]bit.Bit{})
	s := rt.ToString()
	if s == "" {
		t.Error("ToString() should not be empty")
	}
}

func TestRate34Data_AllOnes(t *testing.T) {
	var infoBits [96]bit.Bit
	for i := range infoBits {
		infoBits[i] = 1
	}
	rt, _ := pdu.DecodeRate34Data(infoBits)
	for i := 0; i < 12; i++ {
		if rt.Data[i] != 0xFF {
			t.Errorf("Data[%d] = 0x%02X, want 0xFF", i, rt.Data[i])
		}
	}
}

func TestRate34Data_EncodeDecodeRoundTrip(t *testing.T) {
	original := pdu.Rate34Data{
		Data: [12]byte{0xAB, 0xCD, 0xEF, 0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF, 0x01},
	}
	encoded := pdu.EncodeRate34Data(&original)
	decoded, _ := pdu.DecodeRate34Data(encoded)
	if decoded.Data != original.Data {
		t.Errorf("round-trip failed: got %v, want %v", decoded.Data, original.Data)
	}
}
