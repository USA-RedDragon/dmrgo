package bptc_test

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
)

// encodeSingleBurst calls bptc.EncodeSingleBurst with one bit.Bit per bit.
func encodeSingleBurst(info [11]bit.Bit, oddParity bool) [32]bit.Bit {
	var tx [32]bit.Bit
	bit.PutUint(tx[:], uint64(bptc.EncodeSingleBurst(uint16(bit.Uint(info[:])), oddParity)))
	return tx
}

// decodeSingleBurst calls bptc.DecodeSingleBurst with one bit.Bit per bit.
func decodeSingleBurst(tx [32]bit.Bit, oddParity bool) ([11]bit.Bit, fec.Result) {
	info, res := bptc.DecodeSingleBurst(uint32(bit.Uint(tx[:])), oddParity)
	var out [11]bit.Bit
	bit.PutUint(out[:], uint64(info))
	return out, res
}
