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

// encodeCACH calls bptc.EncodeCACH with one bit.Bit per bit.
func encodeCACH(info [36]bit.Bit) [4][17]bit.Bit {
	var out [4][17]bit.Bit
	for i, f := range bptc.EncodeCACH(bit.Uint(info[:])) {
		bit.PutUint(out[i][:], uint64(f))
	}
	return out
}

// decodeCACH calls bptc.DecodeCACH with one bit.Bit per bit.
func decodeCACH(fragments [4][17]bit.Bit) ([36]bit.Bit, fec.Result) {
	var words [4]uint32
	for i := range fragments {
		words[i] = uint32(bit.Uint(fragments[i][:]) & 0x1FFFF)
	}
	info, res := bptc.DecodeCACH(words)
	var out [36]bit.Bit
	bit.PutUint(out[:], info)
	return out, res
}
