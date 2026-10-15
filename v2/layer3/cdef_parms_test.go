package layer3_test

import (
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func TestCdefParms_NewFromBits(t *testing.T) {
	t.Parallel()

	var bits [58]bit.Bit

	setBitsFromUint16(bits[:], 0, 12, 291)
	setBitsFromUint16(bits[:], 12, 10, 440)
	setBitsFromUint16(bits[:], 22, 13, 4000)
	setBitsFromUint16(bits[:], 35, 10, 445)

	parmsValue := layer3.DecodeCdefParms(bits)
	parms := &parmsValue

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
	t.Parallel()

	parms := &layer3.CdefParms{
		Channel: 1,
		TXMHz:   440,
		TXKHz:   4000, // 4000 steps of 125 Hz is 0.5 MHz
		RXMHz:   445,
		RXKHz:   4000,
	}

	txFreq := parms.TXFrequencyMHz()
	rxFreq := parms.RXFrequencyMHz()

	if math.Abs(txFreq-440.5) > 0.001 {
		t.Errorf("TXFrequencyMHz() = %f, want 440.5", txFreq)
	}
	if math.Abs(rxFreq-445.5) > 0.001 {
		t.Errorf("RXFrequencyMHz() = %f, want 445.5", rxFreq)
	}
	if got := parms.TXFrequencyHz(); got != 440_500_000 {
		t.Errorf("TXFrequencyHz() = %d, want 440500000", got)
	}
	if got := parms.RXFrequencyHz(); got != 445_500_000 {
		t.Errorf("RXFrequencyHz() = %d, want 445500000", got)
	}
}

func TestCdefParms_String(t *testing.T) {
	t.Parallel()

	parms := &layer3.CdefParms{
		Channel: 1,
		TXMHz:   440,
		TXKHz:   0,
		RXMHz:   445,
		RXKHz:   0,
	}

	str := parms.String()
	if str == "" {
		t.Error("String should not be empty")
	}
}

func TestCdefParms_ZeroValues(t *testing.T) {
	t.Parallel()

	var bits [58]bit.Bit
	parmsValue := layer3.DecodeCdefParms(bits)
	parms := &parmsValue

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
