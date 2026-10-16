package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// DecodeEmbeddedRC decodes a Reverse Channel PDU from the 32-bit embedded
// signalling data field. The FEC field of the PDU combines the results of
// the odd parity Single Burst BPTC and the CRC-7 (ETSI TS 102 361-1 clause
// B.2.2, clause B.3.13).
func DecodeEmbeddedRC(embeddedData [32]bit.Bit) pdu.ReverseChannel {
	var d [4]byte
	packBits(d[:], embeddedData[:])
	return decodeRC(d)
}

// decodeRC decodes a Reverse Channel PDU from the 32 packed bits d, as
// DecodeEmbeddedRC does.
func decodeRC(d [4]byte) pdu.ReverseChannel {
	tx := uint32(d[0])<<24 | uint32(d[1])<<16 | uint32(d[2])<<8 | uint32(d[3])
	info, bptcResult := bptc.DecodeSingleBurst(tx, true)
	var infoBits [11]bit.Bit
	bit.PutUint(infoBits[:], uint64(info))
	rc := pdu.DecodeReverseChannel(infoBits)
	rc.FEC = bptcResult.Combine(rc.FEC)
	return rc
}

// EncodeEmbeddedRC encodes a Reverse Channel PDU into the 32 transmit bits
// of the embedded signalling data field. It returns an error when the RC
// command does not fit in 4 bits.
func EncodeEmbeddedRC(rc *pdu.ReverseChannel) ([32]bit.Bit, error) {
	var tx [32]bit.Bit
	d, err := encodeRC(rc)
	if err != nil {
		return tx, err
	}
	unpackBits(tx[:], d[:])
	return tx, nil
}

// encodeRC encodes rc as EncodeEmbeddedRC does and returns the 32 bits
// packed.
func encodeRC(rc *pdu.ReverseChannel) ([4]byte, error) {
	infoBits, err := rc.Encode()
	if err != nil {
		return [4]byte{}, err
	}
	tx := bptc.EncodeSingleBurst(uint16(bit.Uint(infoBits[:])&0x7FF), true)
	return [4]byte{byte(tx >> 24), byte(tx >> 16 & 0xFF), byte(tx >> 8 & 0xFF), byte(tx & 0xFF)}, nil
}
