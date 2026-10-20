package bptc

import (
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

func encode196Bits(data [96]unpacked.Bit) [196]unpacked.Bit {
	return encode196BitsReserved(data, [3]unpacked.Bit{})
}

func encode196BitsReserved(data [96]unpacked.Bit, res [3]unpacked.Bit) [196]unpacked.Bit {
	p := Encode196(unpacked.Pack96(data), uint8(res[0]&1|res[1]&1<<1|res[2]&1<<2))
	var info [196]unpacked.Bit
	unpacked.UnpackAt(info[:], p[:], 0)
	return info
}

func decode196Bits(info [196]unpacked.Bit) ([96]unpacked.Bit, fec.Result) {
	data, _, res := decode196BitsReserved(info)
	return data, res
}

func decode196BitsReserved(info [196]unpacked.Bit) ([96]unpacked.Bit, [3]unpacked.Bit, fec.Result) {
	var p [25]byte
	unpacked.PackAt(p[:], 0, info[:])
	data, r, res := Decode196(p)
	return unpacked.Unpack96(data), [3]unpacked.Bit{unpacked.Bit(r & 1), unpacked.Bit(r >> 1 & 1), unpacked.Bit(r >> 2 & 1)}, res
}

func gridRows(g *[196]unpacked.Bit) [13]uint16 {
	var rows [13]uint16
	for i := 1; i < 196; i++ {
		rows[(i-1)/15] |= uint16(g[i]&1) << ((i - 1) % 15)
	}
	return rows
}
