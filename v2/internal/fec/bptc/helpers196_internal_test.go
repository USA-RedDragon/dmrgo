package bptc

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	packed "github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

// encode196Bits calls Encode196 with one bit.Bit per bit and R(0) to R(2)
// zero.
func encode196Bits(data [96]bit.Bit) [196]bit.Bit {
	return encode196BitsReserved(data, [3]bit.Bit{})
}

// encode196BitsReserved calls Encode196 with one bit.Bit per bit.
func encode196BitsReserved(data [96]bit.Bit, res [3]bit.Bit) [196]bit.Bit {
	p := Encode196(unpacked.Pack96(data), uint8(res[0]&1|res[1]&1<<1|res[2]&1<<2))
	var info [196]bit.Bit
	packed.Unpack(info[:], p[:], 0)
	return info
}

// decode196Bits calls Decode196 with one bit.Bit per bit.
func decode196Bits(info [196]bit.Bit) ([96]bit.Bit, fec.Result) {
	data, _, res := decode196BitsReserved(info)
	return data, res
}

// decode196BitsReserved calls Decode196 with one bit.Bit per bit.
func decode196BitsReserved(info [196]bit.Bit) ([96]bit.Bit, [3]bit.Bit, fec.Result) {
	var p [25]byte
	packed.Pack(p[:], 0, info[:])
	data, r, res := Decode196(p)
	return unpacked.Unpack96(data), [3]bit.Bit{bit.Bit(r & 1), bit.Bit(r >> 1 & 1), bit.Bit(r >> 2 & 1)}, res
}

// gridRows returns the rows of a 196-bit grid, grid index i at row (i-1)/15
// and column (i-1)%15.
func gridRows(g *[196]bit.Bit) [13]uint16 {
	var rows [13]uint16
	for i := 1; i < 196; i++ {
		rows[(i-1)/15] |= uint16(g[i]&1) << ((i - 1) % 15)
	}
	return rows
}
