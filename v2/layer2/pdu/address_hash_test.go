package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func crc8Bytes(data []byte) uint8 {
	var r uint8
	for _, b := range data {
		r ^= b
		for range 8 {
			if r&0x80 != 0 {
				r = r<<1 ^ 0x07
			} else {
				r <<= 1
			}
		}
	}
	return r
}

func TestAddressHash(t *testing.T) {
	t.Parallel()
	if got := crc8Bytes([]byte("123456789")); got != 0xF4 {
		t.Fatalf("reference CRC-8 check value = %#x, want 0xf4", got)
	}
	for _, id := range []uint32{0, 1, 9990, 3191868, 3120001, 0xFFFFFF, 0x123456} {
		want := crc8Bytes([]byte{byte(id >> 16), byte(id >> 8), byte(id)})
		if got := must(pdu.AddressHash(id)); got != want {
			t.Errorf("AddressHash(%d) = %#x, want %#x", id, got, want)
		}
	}
	if _, err := pdu.AddressHash(0x1000001); !isRangeError(err, "pdu", "AddressHash", 24) {
		t.Errorf("AddressHash(0x1000001): %v, want the FieldRangeError of AddressHash", err)
	}
	if got := must(pdu.AddressHash(1)); got != 0x07 {
		t.Errorf("AddressHash(1) = %#x, want 0x07", got)
	}
}
