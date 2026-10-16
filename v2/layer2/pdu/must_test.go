package pdu_test

import "github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"

// rawBit returns bit i of a raw byte-array field, which holds its bits most
// significant bit first.
func rawBit(b []byte, i int) unpacked.Bit {
	return unpacked.Bit(b[i/8] >> (7 - i%8) & 1)
}

// setRawBit sets bit i of a raw byte-array field.
func setRawBit(b []byte, i int) {
	b[i/8] |= 0x80 >> (i % 8)
}

// must returns v and panics on err, for encode calls a test expects to succeed.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
