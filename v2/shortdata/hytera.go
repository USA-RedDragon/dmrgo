package shortdata

// HyteraText is the data of the defined data text that AnyTone radios send
// in H-SMS mode, with SAP short data and DD format 1. Captures of a DA-7X2
// with AnyTone D890UV firmware show it.
//
//dmr:stream
type HyteraText struct {
	// Field is a 16-bit value before the text whose meaning is not known:
	// it was 00 00 in four captures and 45 00 in one. Decode keeps it as
	// it is.
	Field uint16 `dmr:"u16"`
	// Text is the text in UTF-16LE, with a NUL character after it and pad
	// octets after that.
	Text []byte `dmr:"rest,utf16le,nul"`
}
