package bptc_test

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	packed "github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

// encodeSingleBurst calls bptc.EncodeSingleBurst with one bit.Bit per bit.
func encodeSingleBurst(info [11]bit.Bit, oddParity bool) [32]bit.Bit {
	var tx [32]bit.Bit
	unpacked.PutUint(tx[:], uint64(bptc.EncodeSingleBurst(uint16(unpacked.Uint(info[:])), oddParity)))
	return tx
}

// decodeSingleBurst calls bptc.DecodeSingleBurst with one bit.Bit per bit.
func decodeSingleBurst(tx [32]bit.Bit, oddParity bool) ([11]bit.Bit, fec.Result) {
	info, res := bptc.DecodeSingleBurst(uint32(unpacked.Uint(tx[:])), oddParity)
	var out [11]bit.Bit
	unpacked.PutUint(out[:], uint64(info))
	return out, res
}

// encodeCACH calls bptc.EncodeCACH with one bit.Bit per bit.
func encodeCACH(info [36]bit.Bit) [4][17]bit.Bit {
	var out [4][17]bit.Bit
	for i, f := range bptc.EncodeCACH(unpacked.Uint(info[:])) {
		unpacked.PutUint(out[i][:], uint64(f))
	}
	return out
}

// decodeCACH calls bptc.DecodeCACH with one bit.Bit per bit.
func decodeCACH(fragments [4][17]bit.Bit) ([36]bit.Bit, fec.Result) {
	var words [4]uint32
	for i := range fragments {
		words[i] = uint32(unpacked.Uint(fragments[i][:]) & 0x1FFFF)
	}
	info, res := bptc.DecodeCACH(words)
	var out [36]bit.Bit
	unpacked.PutUint(out[:], info)
	return out, res
}

func embeddedWords(fragments [4][32]bit.Bit) [4]uint32 {
	var w [4]uint32
	for i := range fragments {
		w[i] = uint32(unpacked.Uint(fragments[i][:]) & 0xFFFFFFFF)
	}
	return w
}

func unpackEmbeddedInfo(p [10]byte) [77]bit.Bit {
	var info [77]bit.Bit
	packed.Unpack(info[:], p[:], 0)
	return info
}

// encodeEmbedded calls bptc.EncodeEmbedded with one bit.Bit per bit.
func encodeEmbedded(info [77]bit.Bit) [4][32]bit.Bit {
	var p [10]byte
	packed.Pack(p[:], 0, info[:])
	var out [4][32]bit.Bit
	for i, w := range bptc.EncodeEmbedded(p) {
		unpacked.PutUint(out[i][:], uint64(w))
	}
	return out
}

// decodeEmbedded calls bptc.DecodeEmbedded with one bit.Bit per bit.
func decodeEmbedded(fragments [4][32]bit.Bit) ([77]bit.Bit, fec.Result) {
	info, res := bptc.DecodeEmbedded(embeddedWords(fragments))
	return unpackEmbeddedInfo(info), res
}

// decodeEmbeddedStrict calls bptc.DecodeEmbeddedStrict with one bit.Bit per
// bit.
func decodeEmbeddedStrict(fragments [4][32]bit.Bit) ([77]bit.Bit, fec.Result) {
	info, res := bptc.DecodeEmbeddedStrict(embeddedWords(fragments))
	return unpackEmbeddedInfo(info), res
}

// decodeEmbeddedWithCheck calls bptc.DecodeEmbeddedWithCheck with one
// bit.Bit per bit.
func decodeEmbeddedWithCheck(fragments [4][32]bit.Bit, check func([77]bit.Bit) bool) ([77]bit.Bit, fec.Result) {
	info, res := bptc.DecodeEmbeddedWithCheck(embeddedWords(fragments), func(p [10]byte) bool { return check(unpackEmbeddedInfo(p)) })
	return unpackEmbeddedInfo(info), res
}
