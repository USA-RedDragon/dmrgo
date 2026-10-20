package layer2

var cachDeinterleaveTable = [CACHBits]int{
	0, 7, 8, 9, 1, 10, 11, 12, 2, 13, 14, 15, 3, 16, 4, 17, 18, 19, 5, 20, 21, 22, 6, 23,
}

var cachInterleaveTable = [CACHBits]int{
	0, 4, 8, 12, 14, 18, 22, 1, 2, 3, 5, 6, 7, 9, 10, 11, 13, 15, 16, 17, 19, 20, 21, 23,
}

// CACHDeinterleave converts a 24-bit CACH field from transmit order to the
// logical order that DecodeCACH takes: AT, TC, LCSS, Hamming parity, then the
// 17 payload bits. Both hold the bits packed, the first bit in the most
// significant bit of byte 0. (ETSI TS 102 361-1 clause B.4.1, Figure B.9)
func CACHDeinterleave(tx [3]byte) [3]byte {
	var logical [3]byte
	for i, to := range cachDeinterleaveTable {
		logical[to>>3] |= tx[i>>3] >> (7 - i&7) & 1 << (7 - to&7)
	}
	return logical
}

// CACHInterleave converts a 24-bit CACH field from logical order to
// transmit order. It is the inverse of CACHDeinterleave.
func CACHInterleave(logical [3]byte) [3]byte {
	var tx [3]byte
	for i, to := range cachInterleaveTable {
		tx[to>>3] |= logical[i>>3] >> (7 - i&7) & 1 << (7 - to&7)
	}
	return tx
}
