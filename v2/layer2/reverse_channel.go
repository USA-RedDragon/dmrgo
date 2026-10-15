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
	infoBits, bptcResult := bptc.DecodeSingleBurst(embeddedData, true)
	rc := pdu.DecodeReverseChannel(infoBits)
	rc.FEC = bptcResult.Combine(rc.FEC)
	return rc
}

// EncodeEmbeddedRC encodes a Reverse Channel PDU into the 32 transmit bits
// of the embedded signalling data field. It returns an error when the RC
// command does not fit in 4 bits.
func EncodeEmbeddedRC(rc *pdu.ReverseChannel) ([32]bit.Bit, error) {
	infoBits, err := rc.Encode()
	if err != nil {
		return [32]bit.Bit{}, err
	}
	return bptc.EncodeSingleBurst(infoBits, true), nil
}
