package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
)

func fragmentOctets(fr [4][32]bit.Bit) [4][4]byte {
	var out [4][4]byte
	for i := range fr {
		copy(out[i][:], bit.Pack(fr[i][:]))
	}
	return out
}

func fragmentBits(f [4][4]byte) [4][32]bit.Bit {
	var fr [4][32]bit.Bit
	for i := range f {
		copy(fr[i][:], bit.Unpack(f[i][:]))
	}
	return fr
}
