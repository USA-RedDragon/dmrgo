package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestRate1Data_DecodeFromBits(t *testing.T) {
	t.Parallel()

	// Build 96 info bits representing known packed data
	var infoBits [24]byte

	// Set byte 0 = 0xAB: 10101011
	bits.SetBit(infoBits[:], 0, 1)
	bits.SetBit(infoBits[:], 2, 1)
	bits.SetBit(infoBits[:], 4, 1)
	bits.SetBit(infoBits[:], 6, 1)
	bits.SetBit(infoBits[:], 7, 1)

	// Set byte 1 = 0xCD: 11001101
	bits.SetBit(infoBits[:], 8, 1)
	bits.SetBit(infoBits[:], 9, 1)
	bits.SetBit(infoBits[:], 12, 1)
	bits.SetBit(infoBits[:], 13, 1)
	bits.SetBit(infoBits[:], 15, 1)

	rt := pdu.DecodeRate1Data(infoBits)
	if rt.Data[0] != 0xAB {
		t.Errorf("Data[0] = 0x%02X, want 0xAB", rt.Data[0])
	}
	if rt.Data[1] != 0xCD {
		t.Errorf("Data[1] = 0x%02X, want 0xCD", rt.Data[1])
	}
	// Remaining bytes should be zero
	for i := 2; i < 24; i++ {
		if rt.Data[i] != 0 {
			t.Errorf("Data[%d] = 0x%02X, want 0x00", i, rt.Data[i])
		}
	}
}

func TestRate1Data_DataType(t *testing.T) {
	t.Parallel()

	rt := pdu.DecodeRate1Data([24]byte{})
	if rt.DataType() != enums.DataTypeRate1 {
		t.Errorf("DataType() = %d, want DataTypeRate1", rt.DataType())
	}
}

func TestRate1Data_String(t *testing.T) {
	t.Parallel()

	rt := pdu.DecodeRate1Data([24]byte{})
	s := rt.String()
	if s == "" {
		t.Error("String() should not be empty")
	}
}

func TestRate1Data_AllOnes(t *testing.T) {
	t.Parallel()

	var infoBits [24]byte
	for i := range infoBits {
		infoBits[i] = 0xFF
	}
	rt := pdu.DecodeRate1Data(infoBits)
	for i := 0; i < 24; i++ {
		if rt.Data[i] != 0xFF {
			t.Errorf("Data[%d] = 0x%02X, want 0xFF", i, rt.Data[i])
		}
	}
}

func TestRate1Data_EncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()

	original := pdu.Rate1Data{
		Data: [24]byte{0xAB, 0xCD, 0xEF, 0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF, 0x01, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1A, 0x1B},
	}
	encoded := must(original.Encode())
	decoded := pdu.DecodeRate1Data(encoded)
	if decoded.Data != original.Data {
		t.Errorf("round-trip failed: got %v, want %v", decoded.Data, original.Data)
	}
}

func TestRate1Data_FullLengthLastByte(t *testing.T) {
	t.Parallel()

	var infoBits [24]byte
	bits.SetBit(infoBits[:], 191, 1)
	rt := pdu.DecodeRate1Data(infoBits)
	if rt.Data[23] != 0x01 {
		t.Errorf("Data[23] = 0x%02X, want 0x01", rt.Data[23])
	}
}

func TestRate1Data_Confirmed_RoundTrip(t *testing.T) {
	t.Parallel()

	var user [22]byte
	for i := range user {
		user[i] = byte(i*7 + 1)
	}
	var rt pdu.Rate1Data
	rt.SetConfirmed(0x55, user)
	if rt.ConfirmedDBSN() != 0x55 {
		t.Errorf("ConfirmedDBSN() = %d, want %d", rt.ConfirmedDBSN(), 0x55)
	}
	if rt.ConfirmedUserData() != user {
		t.Errorf("ConfirmedUserData() = %X, want %X", rt.ConfirmedUserData(), user)
	}
	if !rt.ConfirmedCRC9OK() {
		t.Fatal("ConfirmedCRC9OK() = false after SetConfirmed")
	}
	decoded := pdu.DecodeRate1Data(must(rt.Encode()))
	if !decoded.ConfirmedCRC9OK() {
		t.Error("ConfirmedCRC9OK() = false after encode/decode")
	}
	decoded.Data[5] ^= 0x10
	if decoded.ConfirmedCRC9OK() {
		t.Error("ConfirmedCRC9OK() = true after corrupting user data")
	}
}

// The CRC-9 covers user data then DBSN (TS 102 361-1 B.3.10) with the
// Table B.21 mask, placed MSB-first in octet 0 bit 0 and octet 1.
func TestRate1Data_Confirmed_CRC9Layout(t *testing.T) {
	t.Parallel()

	var user [22]byte
	user[0] = 0x80
	var rt pdu.Rate1Data
	rt.SetConfirmed(3, user)

	want := (crc.CRC9(append(user[:], 3<<1), len(user)*8+7) ^ 0x10F) & 0x1FF
	if got := rt.ConfirmedCRC9(); got != want {
		t.Errorf("ConfirmedCRC9() = 0x%03X, want 0x%03X", got, want)
	}
	if rt.Data[0] != byte(3<<1)|byte(want>>8) || rt.Data[1] != byte(want) {
		t.Errorf("header octets = %02X %02X, want DBSN<<1|CRC MSB, CRC low", rt.Data[0], rt.Data[1])
	}
	if m, _, _ := pdu.CRCMask(enums.DataTypeRate1); m != 0x10F {
		t.Errorf("CRCMask(Rate1) = 0x%03X, want 0x10F", m)
	}
}
