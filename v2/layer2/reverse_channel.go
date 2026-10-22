package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// DecodeEmbeddedRC decodes a Reverse Channel PDU from the 32-bit embedded
// signalling data field, packed, the first bit in the most significant bit
// of byte 0, as RawBurst.EmbeddedData returns it. The FEC field of the PDU
// combines the results of the odd parity Single Burst BPTC and the CRC-7
// (ETSI TS 102 361-1 clause B.2.2, clause B.3.13).
func DecodeEmbeddedRC(data [4]byte) pdu.ReverseChannel {
	tx := uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
	info, bptcResult := bptc.DecodeSingleBurst(tx, true)
	rc := pdu.DecodeReverseChannel([2]byte{byte(info >> 3 & 0xFF), byte(info << 5 & 0xE0)})
	rc.FEC = bptcResult.Combine(rc.FEC)
	return rc
}

// EncodeEmbeddedRC encodes a Reverse Channel PDU into the 32 transmit bits
// of the embedded signalling data field, packed as DecodeEmbeddedRC takes
// them. It returns an error when the RC command does not fit in 4 bits, and
// dmrgo.ErrNilArgument for a nil rc.
func EncodeEmbeddedRC(rc *pdu.ReverseChannel) ([4]byte, error) {
	if rc == nil {
		return [4]byte{}, dmrgo.ErrNilArgument
	}
	info, err := rc.Encode()
	if err != nil {
		return [4]byte{}, err
	}
	tx := bptc.EncodeSingleBurst(uint16(info[0])<<3|uint16(info[1])>>5, true)
	return [4]byte{byte(tx >> 24), byte(tx >> 16 & 0xFF), byte(tx >> 8 & 0xFF), byte(tx & 0xFF)}, nil
}
