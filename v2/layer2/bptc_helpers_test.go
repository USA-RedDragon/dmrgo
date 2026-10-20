package layer2_test

import (
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

func decode196Bits(info [196]unpacked.Bit) [96]unpacked.Bit {
	var p [25]byte
	unpacked.PackAt(p[:], 0, info[:])
	data, _, _ := bptc.Decode196(p)
	return unpacked.Unpack96(data)
}

func encode196Bits(data [96]unpacked.Bit) [196]unpacked.Bit {
	p := bptc.Encode196(unpacked.Pack96(data), 0)
	var info [196]unpacked.Bit
	unpacked.UnpackAt(info[:], p[:], 0)
	return info
}
