package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/crc"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestRate34Data_DecodeFromBits(t *testing.T) {
	// Build 96 info bits representing known packed data
	var infoBits [144]bit.Bit

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
	for i := 2; i < 18; i++ {
		if rt.Data[i] != 0 {
			t.Errorf("Data[%d] = 0x%02X, want 0x00", i, rt.Data[i])
		}
	}
}

func TestRate34Data_GetDataType(t *testing.T) {
	rt, _ := pdu.DecodeRate34Data([144]bit.Bit{})
	rt.DataType = enums.DataTypeRate34
	if rt.GetDataType() != enums.DataTypeRate34 {
		t.Errorf("GetDataType() = %d, want DataTypeRate34", rt.GetDataType())
	}
}

func TestRate34Data_String(t *testing.T) {
	rt, _ := pdu.DecodeRate34Data([144]bit.Bit{})
	s := rt.String()
	if s == "" {
		t.Error("String() should not be empty")
	}
}

func TestRate34Data_AllOnes(t *testing.T) {
	var infoBits [144]bit.Bit
	for i := range infoBits {
		infoBits[i] = 1
	}
	rt, _ := pdu.DecodeRate34Data(infoBits)
	for i := 0; i < 18; i++ {
		if rt.Data[i] != 0xFF {
			t.Errorf("Data[%d] = 0x%02X, want 0xFF", i, rt.Data[i])
		}
	}
}

func TestRate34Data_EncodeDecodeRoundTrip(t *testing.T) {
	original := pdu.Rate34Data{
		Data: [18]byte{0xAB, 0xCD, 0xEF, 0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF, 0x01, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15},
	}
	encoded := must(pdu.EncodeRate34Data(&original))
	decoded, _ := pdu.DecodeRate34Data(encoded)
	if decoded.Data != original.Data {
		t.Errorf("round-trip failed: got %v, want %v", decoded.Data, original.Data)
	}
}

func TestRate34Data_FullLengthLastByte(t *testing.T) {
	var infoBits [144]bit.Bit
	infoBits[143] = 1
	rt, _ := pdu.DecodeRate34Data(infoBits)
	if rt.Data[17] != 0x01 {
		t.Errorf("Data[17] = 0x%02X, want 0x01", rt.Data[17])
	}
}

func TestRate34Data_Confirmed_RoundTrip(t *testing.T) {
	var user [16]byte
	for i := range user {
		user[i] = byte(i*7 + 1)
	}
	var rt pdu.Rate34Data
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
	decoded, _ := pdu.DecodeRate34Data(must(pdu.EncodeRate34Data(&rt)))
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
func TestRate34Data_Confirmed_CRC9Layout(t *testing.T) {
	var user [16]byte
	user[0] = 0x80
	var rt pdu.Rate34Data
	rt.SetConfirmed(3, user)

	bits := append(bit.UnpackBits(user[:]), bit.BitsFromUint8(3, 7)...)
	want := (crc.CalculateCRC9(bits) ^ 0x1FF) & 0x1FF
	if got := rt.ConfirmedCRC9(); got != want {
		t.Errorf("ConfirmedCRC9() = 0x%03X, want 0x%03X", got, want)
	}
	if rt.Data[0] != byte(3<<1)|byte(want>>8) || rt.Data[1] != byte(want) {
		t.Errorf("header octets = %02X %02X, want DBSN<<1|CRC MSB, CRC low", rt.Data[0], rt.Data[1])
	}
	if pdu.CRC9MaskRate34 != 0x1FF {
		t.Errorf("CRC9MaskRate34 = 0x%03X, want 0x1FF", pdu.CRC9MaskRate34)
	}
}
