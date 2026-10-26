package compute

// PRFillBits returns the 96 pseudo-random fill bits of an idle burst, most
// significant bit of each octet first (ETSI TS 102 361-1 Table D.2).
func PRFillBits() [96]uint8 {
	packed := [12]byte{
		0xFF, 0x83, 0xDF, 0x17, 0x32, 0x09,
		0x4E, 0xD1, 0xE7, 0xCD, 0x8A, 0x91,
	}
	var out [96]uint8
	for i := range out {
		out[i] = packed[i/8] >> (7 - i%8) & 1
	}
	return out
}
