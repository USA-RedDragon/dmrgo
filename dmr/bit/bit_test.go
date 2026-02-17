package bit

import (
	"testing"
)

func TestBitsToUint8(t *testing.T) {
	bits := []Bit{1, 0, 1, 1, 0, 0, 1, 0}
	got := BitsToUint8(bits, 0, 8)
	want := uint8(0b10110010)
	if got != want {
		t.Errorf("BitsToUint8 = %d, want %d", got, want)
	}

	// Partial extraction
	got = BitsToUint8(bits, 2, 4)
	want = uint8(0b1100)
	if got != want {
		t.Errorf("BitsToUint8(partial) = %d, want %d", got, want)
	}
}

func TestBitsToUint16(t *testing.T) {
	// 0xABCD = 1010 1011 1100 1101
	bits := make([]Bit, 16)
	val := uint16(0xABCD)
	for i := 0; i < 16; i++ {
		bits[i] = Bit((val >> (15 - i)) & 1)
	}
	got := BitsToUint16(bits, 0, 16)
	if got != val {
		t.Errorf("BitsToUint16 = %04x, want %04x", got, val)
	}
}

func TestBitsToUint32(t *testing.T) {
	bits := make([]Bit, 24)
	val := uint32(0x123456)
	for i := 0; i < 24; i++ {
		bits[i] = Bit((val >> (23 - i)) & 1)
	}
	got := BitsToUint32(bits, 0, 24)
	if got != val {
		t.Errorf("BitsToUint32 = %06x, want %06x", got, val)
	}
}

func TestBitsToInt(t *testing.T) {
	bits := []Bit{1, 1, 0, 1}
	got := BitsToInt(bits, 0, 4)
	want := 0b1101
	if got != want {
		t.Errorf("BitsToInt = %d, want %d", got, want)
	}
}

func TestBitsToBool(t *testing.T) {
	bits := []Bit{0, 1, 0}
	if BitsToBool(bits, 0) {
		t.Error("expected false for bit 0")
	}
	if !BitsToBool(bits, 1) {
		t.Error("expected true for bit 1")
	}
}

func TestBitsFromUint8(t *testing.T) {
	bits := BitsFromUint8(0b10110010, 8)
	expected := []Bit{1, 0, 1, 1, 0, 0, 1, 0}
	for i := range expected {
		if bits[i] != expected[i] {
			t.Errorf("BitsFromUint8[%d] = %d, want %d", i, bits[i], expected[i])
		}
	}
}

func TestBitsFromUint16(t *testing.T) {
	bits := BitsFromUint16(0xABCD, 16)
	got := BitsToUint16(bits, 0, 16)
	if got != 0xABCD {
		t.Errorf("roundtrip failed: got %04x", got)
	}
}

func TestBitsFromUint32(t *testing.T) {
	bits := BitsFromUint32(0x123456, 24)
	got := BitsToUint32(bits, 0, 24)
	if got != 0x123456 {
		t.Errorf("roundtrip failed: got %06x", got)
	}
}

func TestPackUnpackBits(t *testing.T) {
	original := []Bit{1, 0, 1, 1, 0, 0, 1, 0, 1, 1, 0, 0, 0, 0, 1, 1}
	packed := PackBits(original)
	if len(packed) != 2 {
		t.Fatalf("expected 2 packed bytes, got %d", len(packed))
	}
	if packed[0] != 0xB2 || packed[1] != 0xC3 {
		t.Errorf("packed = %02x %02x, want B2 C3", packed[0], packed[1])
	}
	unpacked := UnpackBits(packed)
	if len(unpacked) != 16 {
		t.Fatalf("expected 16 unpacked bits, got %d", len(unpacked))
	}
	for i := range original {
		if unpacked[i] != original[i] {
			t.Errorf("unpacked[%d] = %d, want %d", i, unpacked[i], original[i])
		}
	}
}

func TestUnpackBytesToBits264_PackBits264_Roundtrip(t *testing.T) {
	var data [33]byte
	data[0] = 0xAB
	data[1] = 0xCD
	data[32] = 0x12
	bits := UnpackBytesToBits264(data)
	packed := PackBits264(bits)
	if packed != data {
		t.Errorf("264-bit roundtrip failed")
	}
}

func TestPackBitsPartialByte(t *testing.T) {
	bits := []Bit{1, 0, 1}
	packed := PackBits(bits)
	if len(packed) != 1 {
		t.Fatalf("expected 1 byte, got %d", len(packed))
	}
	// 1 0 1 _ _ _ _ _ = 0xA0
	if packed[0] != 0xA0 {
		t.Errorf("packed = %02x, want A0", packed[0])
	}
}
