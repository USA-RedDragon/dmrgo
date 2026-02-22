package elements_test

import (
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/layer3/elements"
)

func TestCdefParms_NewFromBits(t *testing.T) {
	// Construct a 58-bit array with known values:
	// Channel (12 bits) = 0x123 = 291
	// TXMHz (10 bits) = 440 = 0b0110111000
	// TXKHz (13 bits) = 4000 = 500 MHz offset → 0.5 MHz = 0b0111110100000
	// RXMHz (10 bits) = 445 = 0b0110111101
	// RXKHz (13 bits) = 0 = 0b0000000000000
	var bits [58]bit.Bit

	// Channel = 291 = 0b000100100011
	setBitsFromUint16(bits[:], 0, 12, 291)
	// TXMHz = 440 = 0b0110111000
	setBitsFromUint16(bits[:], 12, 10, 440)
	// TXKHz = 4000 = 0b0111110100000
	setBitsFromUint16(bits[:], 22, 13, 4000)
	// RXMHz = 445
	setBitsFromUint16(bits[:], 35, 10, 445)
	// RXKHz = 0
	// (already zero)

	parms := elements.NewCdefParmsFromBits(bits)

	if parms.Channel != 291 {
		t.Errorf("Channel = %d, want 291", parms.Channel)
	}
	if parms.TXMHz != 440 {
		t.Errorf("TXMHz = %d, want 440", parms.TXMHz)
	}
	if parms.TXKHz != 4000 {
		t.Errorf("TXKHz = %d, want 4000", parms.TXKHz)
	}
	if parms.RXMHz != 445 {
		t.Errorf("RXMHz = %d, want 445", parms.RXMHz)
	}
	if parms.RXKHz != 0 {
		t.Errorf("RXKHz = %d, want 0", parms.RXKHz)
	}
}

func TestCdefParms_FrequencyCalculation(t *testing.T) {
	parms := &elements.CdefParms{
		Channel: 1,
		TXMHz:   440,
		TXKHz:   4000, // 4000 × 0.000125 = 0.5 MHz
		RXMHz:   445,
		RXKHz:   4000, // 4000 × 0.000125 = 0.5 MHz
	}

	txFreq := parms.TXFrequencyMHz()
	rxFreq := parms.RXFrequencyMHz()

	if math.Abs(txFreq-440.5) > 0.001 {
		t.Errorf("TXFrequencyMHz() = %f, want 440.5", txFreq)
	}
	if math.Abs(rxFreq-445.5) > 0.001 {
		t.Errorf("RXFrequencyMHz() = %f, want 445.5", rxFreq)
	}
}

func TestCdefParms_ToString(t *testing.T) {
	parms := &elements.CdefParms{
		Channel: 1,
		TXMHz:   440,
		TXKHz:   0,
		RXMHz:   445,
		RXKHz:   0,
	}

	str := parms.ToString()
	if str == "" {
		t.Error("ToString should not be empty")
	}
}

func TestCdefParms_ZeroValues(t *testing.T) {
	var bits [58]bit.Bit
	parms := elements.NewCdefParmsFromBits(bits)

	if parms.Channel != 0 {
		t.Errorf("Channel = %d, want 0", parms.Channel)
	}
	if parms.TXFrequencyMHz() != 0 {
		t.Errorf("TXFrequencyMHz() = %f, want 0", parms.TXFrequencyMHz())
	}
	if parms.RXFrequencyMHz() != 0 {
		t.Errorf("RXFrequencyMHz() = %f, want 0", parms.RXFrequencyMHz())
	}
}

// setBitsFromUint16 writes a value into a bit slice at the given offset and width.
func setBitsFromUint16(bits []bit.Bit, offset, width int, val uint16) {
	for i := 0; i < width; i++ {
		if (val>>(width-1-i))&1 == 1 {
			bits[offset+i] = 1
		} else {
			bits[offset+i] = 0
		}
	}
}
