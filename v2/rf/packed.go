package rf

import (
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// cachBytes packs the 24 bits of a CACH field, one unpacked.Bit per bit, in the
// form that the layer2 CACH functions take.
func cachBytes(b [layer2.CACHBits]unpacked.Bit) [3]byte {
	var p [3]byte
	for i, v := range b {
		p[i>>3] |= byte(v&1) << (7 - i&7)
	}
	return p
}
