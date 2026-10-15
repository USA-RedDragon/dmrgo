package layer2

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
// The constants give the field bounds in transmit bit order; RawBurst reads
// and writes the fields.

const (
	// BurstBits is the total number of bits in a DMR burst.
	BurstBits = 264

	// infoFirstStart is the start bit index of the first info half.
	infoFirstStart = 0

	// infoFirstEnd is the exclusive end bit index of the first info half.
	infoFirstEnd = 98

	// slotTypeFirstStart is the start bit index of the first slot type half.
	slotTypeFirstStart = 98

	// slotTypeFirstEnd is the exclusive end bit index of the first slot type half.
	slotTypeFirstEnd = 108

	// syncStart is the start bit index of the SYNC/EMB field.
	syncStart = 108

	// syncEnd is the exclusive end bit index of the SYNC/EMB field.
	syncEnd = 156

	// slotTypeSecondStart is the start bit index of the second slot type half.
	slotTypeSecondStart = 156

	// slotTypeSecondEnd is the exclusive end bit index of the second slot type half.
	slotTypeSecondEnd = 166

	// infoSecondStart is the start bit index of the second info half.
	infoSecondStart = 166

	// infoSecondEnd is the exclusive end bit index of the second info half.
	infoSecondEnd = 264

	// embFirstStart is the start of the first EMB half (voice bursts with embedded signalling).
	embFirstStart = 108

	// embFirstEnd is the exclusive end of the first EMB half.
	embFirstEnd = 116

	// embeddedDataStart is the start of the 32-bit embedded data field.
	embeddedDataStart = 116

	// embeddedDataEnd is the exclusive end of the 32-bit embedded data field.
	embeddedDataEnd = 148

	// embSecondStart is the start of the second EMB half.
	embSecondStart = 148

	// embSecondEnd is the exclusive end of the second EMB half.
	embSecondEnd = 156

	// The standalone inbound RC burst (§6.4.1, Figure 6.7, Table E.12)
	// places EMB(8) RC(16) SYNC(48) RC(16) EMB(8) around the burst centre.

	// rcembFirstStart is the start of the first EMB half in a standalone RC burst.
	rcembFirstStart = 84

	// rcembFirstEnd is the exclusive end of the first EMB half in a standalone RC burst.
	rcembFirstEnd = 92

	// rcDataFirstStart is the start of the first RC data half in a standalone RC burst.
	rcDataFirstStart = 92

	// rcDataFirstEnd is the exclusive end of the first RC data half in a standalone RC burst.
	rcDataFirstEnd = 108

	// rcDataSecondStart is the start of the second RC data half in a standalone RC burst.
	rcDataSecondStart = 156

	// rcDataSecondEnd is the exclusive end of the second RC data half in a standalone RC burst.
	rcDataSecondEnd = 172

	// rcembSecondStart is the start of the second EMB half in a standalone RC burst.
	rcembSecondStart = 172

	// rcembSecondEnd is the exclusive end of the second EMB half in a standalone RC burst.
	rcembSecondEnd = 180
)
