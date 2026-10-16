package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// TestIdleMessageSoftBits checks that IsIdleMessage reads only the low bit
// of each value.
func TestIdleMessageSoftBits(t *testing.T) {
	t.Parallel()

	im := layer2.IdleMessageInfo()
	for i := range im {
		im[i] |= 0xFE
	}
	if !layer2.IsIdleMessage(im) {
		t.Error("IsIdleMessage is false for the idle message with high bits set")
	}
	im[5] ^= 1
	if layer2.IsIdleMessage(im) {
		t.Error("IsIdleMessage is true for a changed idle message")
	}
}

// TestCACHEncodeSoftBits checks that CACH.Encode writes only 0 and 1 for a
// payload with high bits set.
func TestCACHEncodeSoftBits(t *testing.T) {
	t.Parallel()

	var c layer2.CACH
	for i := range c.Payload {
		c.Payload[i] = bit.Bit(0xD0 | i&1)
	}
	bits, err := c.Encode()
	if err != nil {
		t.Fatal(err)
	}
	for i, b := range bits[layer2.TACTBits:] {
		if b != bit.Bit(i&1) {
			t.Fatalf("payload bit %d = %#x, want %d", i, b, i&1)
		}
	}
}
