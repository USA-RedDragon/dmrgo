package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
)

// ETSI TS 102 361-1 Annex E — Transmit Bit Order
//
// A DMR burst consists of 132 dibit symbols transmitted serially:
//
//	L66, L65, ..., L1, R1, R2, ..., R66
//
// Each symbol carries 2 bits (Bit 1 = MSB, Bit 0 = LSB), giving 264 bits total.
// The 33-byte input to NewBurstFromBytes is already in transmit bit order.
//
// The standard burst layout for data bursts is (Annex E, Table E.1):
//
//	Bits [0..97]   — Info(0..97)   (first data half, 98 bits)
//	Bits [98..107]  — SlotType first half (10 bits: Golay(20,8) encoded CC+DT)
//	Bits [108..155] — SYNC pattern or EMB+ES (48 bits)
//	Bits [156..165] — SlotType second half (10 bits)
//	Bits [166..263] — Info(98..195) (second data half, 98 bits)
//
// This package documents the transmit order per the spec. Since the library
// already receives data in transmit order (33 bytes), these constants and
// functions serve as documentation and validation aids.

const (
	// BurstBits is the total number of bits in a DMR burst.
	BurstBits = 264

	// BurstSymbols is the total number of dibit symbols in a DMR burst.
	BurstSymbols = 132

	// InfoFirstStart is the start bit index of the first info half.
	InfoFirstStart = 0

	// InfoFirstEnd is the exclusive end bit index of the first info half.
	InfoFirstEnd = 98

	// SlotTypeFirstStart is the start bit index of the first slot type half.
	SlotTypeFirstStart = 98

	// SlotTypeFirstEnd is the exclusive end bit index of the first slot type half.
	SlotTypeFirstEnd = 108

	// SyncStart is the start bit index of the SYNC/EMB field.
	SyncStart = 108

	// SyncEnd is the exclusive end bit index of the SYNC/EMB field.
	SyncEnd = 156

	// SlotTypeSecondStart is the start bit index of the second slot type half.
	SlotTypeSecondStart = 156

	// SlotTypeSecondEnd is the exclusive end bit index of the second slot type half.
	SlotTypeSecondEnd = 166

	// InfoSecondStart is the start bit index of the second info half.
	InfoSecondStart = 166

	// InfoSecondEnd is the exclusive end bit index of the second info half.
	InfoSecondEnd = 264

	// EMBFirstStart is the start of the first EMB half (voice bursts with embedded signalling).
	EMBFirstStart = 108

	// EMBFirstEnd is the exclusive end of the first EMB half.
	EMBFirstEnd = 116

	// EmbeddedDataStart is the start of the 32-bit embedded data field.
	EmbeddedDataStart = 116

	// EmbeddedDataEnd is the exclusive end of the 32-bit embedded data field.
	EmbeddedDataEnd = 148

	// EMBSecondStart is the start of the second EMB half.
	EMBSecondStart = 148

	// EMBSecondEnd is the exclusive end of the second EMB half.
	EMBSecondEnd = 156
)

// DibitSymbol represents a single dibit symbol from the burst.
// A dibit encodes two bits: Bit1 (MSB) and Bit0 (LSB).
type DibitSymbol struct {
	// Index is the linear dibit index (0..131) in transmission order.
	Index int
	// Label is the symbol label per Annex E (e.g. "L66", "L1", "R1", "R66").
	Label string
	// Bit1 is the MSB (first bit) of the dibit.
	Bit1 bit.Bit
	// Bit0 is the LSB (second bit) of the dibit.
	Bit0 bit.Bit
}

// BurstToDibits converts a 264-bit burst into 132 dibit symbols in
// transmission order (L66 through R66).
func BurstToDibits(bits [264]bit.Bit) [132]DibitSymbol {
	var symbols [132]DibitSymbol
	for i := 0; i < 132; i++ {
		symbols[i] = DibitSymbol{
			Index: i,
			Label: dibitLabel(i),
			Bit1:  bits[i*2],
			Bit0:  bits[i*2+1],
		}
	}
	return symbols
}

// DibitsToBurst converts 132 dibit symbols back to a 264-bit burst.
func DibitsToBurst(symbols [132]DibitSymbol) [264]bit.Bit {
	var bits [264]bit.Bit
	for i := 0; i < 132; i++ {
		bits[i*2] = symbols[i].Bit1
		bits[i*2+1] = symbols[i].Bit0
	}
	return bits
}

// ValidateBurstStructure checks that the 264-bit burst has all bits in valid
// range (0 or 1). Returns the index of the first invalid bit, or -1 if valid.
func ValidateBurstStructure(bits [264]bit.Bit) int {
	for i, b := range bits {
		if b != 0 && b != 1 {
			return i
		}
	}
	return -1
}

// dibitLabel returns the Annex E symbol label for the given dibit index.
// Indices 0..65 map to L66..L1, indices 66..131 map to R1..R66.
func dibitLabel(idx int) string {
	if idx < 66 {
		n := 66 - idx
		return "L" + itoa(n)
	}
	n := idx - 65
	return "R" + itoa(n)
}

// itoa is a minimal int-to-string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [3]byte{} // max 3 digits (max 132)
	pos := 2
	for n > 0 {
		buf[pos] = byte('0' + n%10) //nolint:gosec // n%10 always in [0,9], fits in byte
		n /= 10
		pos--
	}
	return string(buf[pos+1:])
}
