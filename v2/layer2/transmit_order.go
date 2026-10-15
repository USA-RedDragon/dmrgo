package layer2

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
