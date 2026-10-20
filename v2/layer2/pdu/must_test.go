package pdu_test

import (
	"errors"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

func rawBit(b []byte, i int) unpacked.Bit {
	return unpacked.Bit(b[i/8] >> (7 - i%8) & 1)
}

func setRawBit(b []byte, i int) {
	b[i/8] |= 0x80 >> (i % 8)
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func isRangeError(err error, typ, field string, n int) bool {
	var rangeErr *dmrgo.FieldRangeError
	return errors.As(err, &rangeErr) && rangeErr.Type == typ && rangeErr.Field == field && rangeErr.Bits == n
}
