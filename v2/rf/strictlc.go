package rf

import (
	"encoding/binary"

	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
)

func strictEmbeddedLC(f [4][4]byte) ([9]byte, bool) {
	var w [4]uint32
	for i := range f {
		w[i] = binary.BigEndian.Uint32(f[i][:])
	}
	info, res := bptc.DecodeEmbeddedStrict(w)
	var lc [9]byte
	var cs uint8
	bits.SetField(lc[:], 0, 22, bits.Field(info[:], 0, 22))
	for row := 2; row < 7; row++ {
		bits.SetField(lc[:], 22+10*(row-2), 10, bits.Field(info[:], row*11, 10))
		cs = cs<<1 | bits.Bit(info[:], row*11+10)
	}
	return lc, !res.Uncorrectable && crc.CheckChecksum5(lc, cs)
}
