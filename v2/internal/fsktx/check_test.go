//go:build !tinygo

package fsktx

import (
	"encoding/binary"
	"math"
)

func check(_ int, err error) {
	if err != nil {
		panic(err)
	}
}

func cf32(x []complex64) []byte {
	b := make([]byte, 0, 8*len(x))
	for _, v := range x {
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(real(v)))
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(imag(v)))
	}
	return b
}
