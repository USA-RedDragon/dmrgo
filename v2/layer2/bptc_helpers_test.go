package layer2_test

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	packedbits "github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
)

// decode196Bits calls bptc.Decode196 with one bit.Bit per bit.
func decode196Bits(info [196]bit.Bit) [96]bit.Bit {
	var p [25]byte
	packedbits.Pack(p[:], 0, info[:])
	data, _, _ := bptc.Decode196(p)
	return bit.Unpack96(data)
}

// encode196Bits calls bptc.Encode196 with one bit.Bit per bit.
func encode196Bits(data [96]bit.Bit) [196]bit.Bit {
	p := bptc.Encode196(bit.Pack96(data), 0)
	var info [196]bit.Bit
	packedbits.Unpack(info[:], p[:], 0)
	return info
}
