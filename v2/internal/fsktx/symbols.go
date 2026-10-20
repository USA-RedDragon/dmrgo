package fsktx

// Dibits writes to dst the 4FSK symbols of the bits of b, two bits per
// symbol from the most significant bit of each byte, by the dibit map: 01 is
// +3, 00 is +1, 10 is -1 and 11 is -3. The 33 bytes of a burst give its 132
// symbols, the 3 bytes of a CACH its 12. Dibits writes the symbols of as many
// bytes of b as fit in dst, all of them when len(dst) is at least 4*len(b),
// and returns them. (ETSI TS 102 361-1 Table 10.3)
func Dibits(dst []int8, b []byte) []int8 {
	b = b[:min(len(b), len(dst)/4)]
	dst = dst[:4*len(b)]
	for i, v := range b {
		for j := range 4 {
			d := int8(v >> uint(6-2*j) & 3)
			dst[4*i+j] = (1 + 2*(d&1)) * (1 - d&2)
		}
	}
	return dst
}
