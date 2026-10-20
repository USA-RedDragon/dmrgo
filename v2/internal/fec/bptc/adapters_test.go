package bptc_test

import (
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

func encodeSingleBurst(info [11]unpacked.Bit, oddParity bool) [32]unpacked.Bit {
	var tx [32]unpacked.Bit
	unpacked.PutUint(tx[:], uint64(bptc.EncodeSingleBurst(uint16(unpacked.Uint(info[:])), oddParity)))
	return tx
}

func decodeSingleBurst(tx [32]unpacked.Bit, oddParity bool) ([11]unpacked.Bit, fec.Result) {
	info, res := bptc.DecodeSingleBurst(uint32(unpacked.Uint(tx[:])), oddParity)
	var out [11]unpacked.Bit
	unpacked.PutUint(out[:], uint64(info))
	return out, res
}

func encodeCACH(info [36]unpacked.Bit) [4][17]unpacked.Bit {
	var out [4][17]unpacked.Bit
	for i, f := range bptc.EncodeCACH(unpacked.Uint(info[:])) {
		unpacked.PutUint(out[i][:], uint64(f))
	}
	return out
}

func decodeCACH(fragments [4][17]unpacked.Bit) ([36]unpacked.Bit, fec.Result) {
	var words [4]uint32
	for i := range fragments {
		words[i] = uint32(unpacked.Uint(fragments[i][:]) & 0x1FFFF)
	}
	info, res := bptc.DecodeCACH(words)
	var out [36]unpacked.Bit
	unpacked.PutUint(out[:], info)
	return out, res
}

func embeddedWords(fragments [4][32]unpacked.Bit) [4]uint32 {
	var w [4]uint32
	for i := range fragments {
		w[i] = uint32(unpacked.Uint(fragments[i][:]) & 0xFFFFFFFF)
	}
	return w
}

func unpackEmbeddedInfo(p [10]byte) [77]unpacked.Bit {
	var info [77]unpacked.Bit
	unpacked.UnpackAt(info[:], p[:], 0)
	return info
}

func encodeEmbedded(info [77]unpacked.Bit) [4][32]unpacked.Bit {
	var p [10]byte
	unpacked.PackAt(p[:], 0, info[:])
	var out [4][32]unpacked.Bit
	for i, w := range bptc.EncodeEmbedded(p) {
		unpacked.PutUint(out[i][:], uint64(w))
	}
	return out
}

func decodeEmbedded(fragments [4][32]unpacked.Bit) ([77]unpacked.Bit, fec.Result) {
	info, res := bptc.DecodeEmbedded(embeddedWords(fragments))
	return unpackEmbeddedInfo(info), res
}

func decodeEmbeddedStrict(fragments [4][32]unpacked.Bit) ([77]unpacked.Bit, fec.Result) {
	info, res := bptc.DecodeEmbeddedStrict(embeddedWords(fragments))
	return unpackEmbeddedInfo(info), res
}

func decodeEmbeddedWithCheck(fragments [4][32]unpacked.Bit, check func([77]unpacked.Bit) bool) ([77]unpacked.Bit, fec.Result) {
	info, res := bptc.DecodeEmbeddedWithCheck(embeddedWords(fragments), func(p [10]byte) bool { return check(unpackEmbeddedInfo(p)) })
	return unpackEmbeddedInfo(info), res
}
