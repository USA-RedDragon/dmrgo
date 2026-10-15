package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// EmbeddedLCFragments is EncodeEmbeddedLC on octets: it returns the four
// embedded signalling fields, in burst order B to E, that carry the 72-bit
// LC lc, in the form RawBurst.SetEmbeddedData takes.
func EmbeddedLCFragments(lc [9]byte) [4][4]byte {
	var bits [72]bit.Bit
	copy(bits[:], bit.Unpack(lc[:]))
	return fragmentOctets(EncodeEmbeddedLC(bits))
}

// DecodeEmbeddedLCFragments is DecodeEmbeddedLC on octets: f holds the
// embedded signalling fields of bursts B to E as RawBurst.EmbeddedData
// returns them.
func DecodeEmbeddedLCFragments(f [4][4]byte) ([9]byte, fec.Result) {
	lc, res := DecodeEmbeddedLC(fragmentBits(f))
	return lcOctets(lc), res
}

// DecodeEmbeddedLCFragmentsStrict is DecodeEmbeddedLCStrict on octets.
func DecodeEmbeddedLCFragmentsStrict(f [4][4]byte) ([9]byte, fec.Result) {
	lc, res := DecodeEmbeddedLCStrict(fragmentBits(f))
	return lcOctets(lc), res
}

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
