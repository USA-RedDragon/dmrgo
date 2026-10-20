package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestRate34Data_DecodeFromBits(t *testing.T) {
	t.Parallel()

	var infoBits [18]byte

	bits.SetBit(infoBits[:], 0, 1)
	bits.SetBit(infoBits[:], 2, 1)
	bits.SetBit(infoBits[:], 4, 1)
	bits.SetBit(infoBits[:], 6, 1)
	bits.SetBit(infoBits[:], 7, 1)

	bits.SetBit(infoBits[:], 8, 1)
	bits.SetBit(infoBits[:], 9, 1)
	bits.SetBit(infoBits[:], 12, 1)
	bits.SetBit(infoBits[:], 13, 1)
	bits.SetBit(infoBits[:], 15, 1)

	rt := pdu.DecodeRate34Data(infoBits)
	if rt.Data[0] != 0xAB {
		t.Errorf("Data[0] = 0x%02X, want 0xAB", rt.Data[0])
	}
	if rt.Data[1] != 0xCD {
		t.Errorf("Data[1] = 0x%02X, want 0xCD", rt.Data[1])
	}
	for i := 2; i < 18; i++ {
		if rt.Data[i] != 0 {
			t.Errorf("Data[%d] = 0x%02X, want 0x00", i, rt.Data[i])
		}
	}
}

func TestRate34Data_DataType(t *testing.T) {
	t.Parallel()

	rt := pdu.DecodeRate34Data([18]byte{})
	if rt.DataType() != enums.DataTypeRate34 {
		t.Errorf("DataType() = %d, want DataTypeRate34", rt.DataType())
	}
}

func TestRate34Data_String(t *testing.T) {
	t.Parallel()

	rt := pdu.DecodeRate34Data([18]byte{})
	s := rt.String()
	if s == "" {
		t.Error("String() should not be empty")
	}
}

func TestRate34Data_AllOnes(t *testing.T) {
	t.Parallel()

	var infoBits [18]byte
	for i := range infoBits {
		infoBits[i] = 0xFF
	}
	rt := pdu.DecodeRate34Data(infoBits)
	for i := 0; i < 18; i++ {
		if rt.Data[i] != 0xFF {
			t.Errorf("Data[%d] = 0x%02X, want 0xFF", i, rt.Data[i])
		}
	}
}

func TestRate34Data_EncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()

	original := pdu.Rate34Data{
		Data: [18]byte{0xAB, 0xCD, 0xEF, 0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF, 0x01, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15},
	}
	encoded := must(original.Encode())
	decoded := pdu.DecodeRate34Data(encoded)
	if decoded.Data != original.Data {
		t.Errorf("round-trip failed: got %v, want %v", decoded.Data, original.Data)
	}
}

func TestRate34Data_FullLengthLastByte(t *testing.T) {
	t.Parallel()

	var infoBits [18]byte
	bits.SetBit(infoBits[:], 143, 1)
	rt := pdu.DecodeRate34Data(infoBits)
	if rt.Data[17] != 0x01 {
		t.Errorf("Data[17] = 0x%02X, want 0x01", rt.Data[17])
	}
}

func TestRate34Data_Confirmed_RoundTrip(t *testing.T) {
	t.Parallel()

	var user [16]byte
	for i := range user {
		user[i] = byte(i*7 + 1)
	}
	var rt pdu.Rate34Data
	noErr(rt.SetConfirmed(0x55, user))
	if rt.ConfirmedDBSN() != 0x55 {
		t.Errorf("ConfirmedDBSN() = %d, want %d", rt.ConfirmedDBSN(), 0x55)
	}
	if rt.ConfirmedUserData() != user {
		t.Errorf("ConfirmedUserData() = %X, want %X", rt.ConfirmedUserData(), user)
	}
	if !rt.ConfirmedCRC9OK() {
		t.Fatal("ConfirmedCRC9OK() = false after SetConfirmed")
	}
	decoded := pdu.DecodeRate34Data(must(rt.Encode()))
	if !decoded.ConfirmedCRC9OK() {
		t.Error("ConfirmedCRC9OK() = false after encode/decode")
	}
	decoded.Data[5] ^= 0x10
	if decoded.ConfirmedCRC9OK() {
		t.Error("ConfirmedCRC9OK() = true after corrupting user data")
	}
}

// TS 102 361-1; Table B.21
func TestRate34Data_Confirmed_CRC9Layout(t *testing.T) {
	t.Parallel()

	var user [16]byte
	user[0] = 0x80
	var rt pdu.Rate34Data
	noErr(rt.SetConfirmed(3, user))

	want := (crc.CRC9(append(user[:], 3<<1), len(user)*8+7) ^ 0x1FF) & 0x1FF
	if got := rt.ConfirmedCRC9(); got != want {
		t.Errorf("ConfirmedCRC9() = 0x%03X, want 0x%03X", got, want)
	}
	if rt.Data[0] != byte(3<<1)|byte(want>>8) || rt.Data[1] != byte(want) {
		t.Errorf("header octets = %02X %02X, want DBSN<<1|CRC MSB, CRC low", rt.Data[0], rt.Data[1])
	}
	if m, _, _ := pdu.CRCMask(enums.DataTypeRate34); m != 0x1FF {
		t.Errorf("CRCMask(Rate34) = 0x%03X, want 0x1FF", m)
	}
}
