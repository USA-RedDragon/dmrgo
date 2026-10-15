package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/crc"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestRate12Data_DecodeFromBits(t *testing.T) {
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

	rt, _ := pdu.DecodeRate12Data(infoBits)
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

func TestRate12Data_GetDataType(t *testing.T) {
	rt, _ := pdu.DecodeRate12Data([96]bit.Bit{})
	rt.DataType = enums.DataTypeRate12
	if rt.GetDataType() != enums.DataTypeRate12 {
		t.Errorf("GetDataType() = %d, want DataTypeRate12", rt.GetDataType())
	}
}

func TestRate12Data_ToString(t *testing.T) {
	rt, _ := pdu.DecodeRate12Data([96]bit.Bit{})
	s := rt.ToString()
	if s == "" {
		t.Error("ToString() should not be empty")
	}
}

func TestRate12Data_AllOnes(t *testing.T) {
	var infoBits [96]bit.Bit
	for i := range infoBits {
		infoBits[i] = 1
	}
	rt, _ := pdu.DecodeRate12Data(infoBits)
	for i := 0; i < 12; i++ {
		if rt.Data[i] != 0xFF {
			t.Errorf("Data[%d] = 0x%02X, want 0xFF", i, rt.Data[i])
		}
	}
}

func TestRate12Data_EncodeDecodeRoundTrip(t *testing.T) {
	original := pdu.Rate12Data{
		Data: [12]byte{0xAB, 0xCD, 0xEF, 0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF, 0x01},
	}
	encoded := must(pdu.EncodeRate12Data(&original))
	decoded, _ := pdu.DecodeRate12Data(encoded)
	if decoded.Data != original.Data {
		t.Errorf("round-trip failed: got %v, want %v", decoded.Data, original.Data)
	}
}

func TestRate12Data_Confirmed_RoundTrip(t *testing.T) {
	var user [10]byte
	for i := range user {
		user[i] = byte(i*7 + 1)
	}
	var rt pdu.Rate12Data
	rt.SetConfirmed(0x55, user)
	if rt.ConfirmedDBSN() != 0x55 {
		t.Errorf("ConfirmedDBSN() = %d, want %d", rt.ConfirmedDBSN(), 0x55)
	}
	if rt.ConfirmedUserData() != user {
		t.Errorf("ConfirmedUserData() = %X, want %X", rt.ConfirmedUserData(), user)
	}
	if !rt.CheckConfirmedCRC9() {
		t.Fatal("CheckConfirmedCRC9() = false after SetConfirmed")
	}
	decoded, _ := pdu.DecodeRate12Data(must(pdu.EncodeRate12Data(&rt)))
	if !decoded.CheckConfirmedCRC9() {
		t.Error("CheckConfirmedCRC9() = false after encode/decode")
	}
	decoded.Data[5] ^= 0x10
	if decoded.CheckConfirmedCRC9() {
		t.Error("CheckConfirmedCRC9() = true after corrupting user data")
	}
}

// The CRC-9 covers user data then DBSN (TS 102 361-1 B.3.10) with the
// Table B.21 mask, placed MSB-first in octet 0 bit 0 and octet 1.
func TestRate12Data_Confirmed_CRC9Layout(t *testing.T) {
	var user [10]byte
	user[0] = 0x80
	var rt pdu.Rate12Data
	rt.SetConfirmed(3, user)

	bits := append(bit.UnpackBits(user[:]), bit.BitsFromUint8(3, 7)...)
	want := (crc.CalculateCRC9(bits) ^ 0x0F0) & 0x1FF
	if got := rt.ConfirmedCRC9(); got != want {
		t.Errorf("ConfirmedCRC9() = 0x%03X, want 0x%03X", got, want)
	}
	if rt.Data[0] != byte(3<<1)|byte(want>>8) || rt.Data[1] != byte(want) {
		t.Errorf("header octets = %02X %02X, want DBSN<<1|CRC MSB, CRC low", rt.Data[0], rt.Data[1])
	}
	if pdu.CRC9MaskRate12 != 0x0F0 {
		t.Errorf("CRC9MaskRate12 = 0x%03X, want 0x0F0", pdu.CRC9MaskRate12)
	}
}
