package layer2_test

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/hamming"
)

// The helpers run the packed internal codecs on bit.Bit values.

func crc7Bits(bits []bit.Bit) uint8 { return crc.CRC7(bit.Pack(bits), len(bits)) }
func crc8Bits(bits []bit.Bit) uint8 { return crc.CRC8(bit.Pack(bits), len(bits)) }

func hamming743Bits(data [4]bit.Bit) [7]bit.Bit {
	var cw [7]bit.Bit
	bit.PutUint(cw[:], uint64(hamming.Encode743(uint8(bit.Uint(data[:])))))
	return cw
}
