package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// ETSI TS 102 361-1 §6.4 — Reverse Channel
//
// The Reverse Channel (RC) provides inbound signalling from an MS to a BS
// during an active voice call. RC data is carried in the 32-bit embedded
// signalling data field of a voice burst.
//
// Two forms:
//   - §6.4.1 Standalone inbound RC: MS sends a burst with the
//     MsSourcedRcSync pattern in the 48-bit centre. EMB (8+8 bits) and RC
//     data (16+16 bits) sit on either side of the SYNC (Figure 6.7).
//   - §6.4.2 Outbound embedded RC: BS echoes RC in a normal voice burst
//     with EMB PI=1 and LCSS=SingleFragmentLCorCSBK.
//
// Detection of §6.4.2: a voice burst's embedded signalling has
// PreemptionAndPowerControlIndicator=true (PI=1) AND
// LCSS=SingleFragmentLCorCSBK (0) → the 32-bit data is RC, not LC.
//
// Processing: 32 transmit bits → Single Burst BPTC (oddParity=true) →
// 11 info bits → RC PDU decode (4 payload + 7 CRC-7 with mask 0x7A).

// DecodeEmbeddedRC decodes a Reverse Channel PDU from the 32-bit
// embedded signalling data field. It applies Single Burst Variable Length
// BPTC with odd parity (§B.2.2) followed by CRC-7 verification (§B.3.13).
//
// The FEC field of the RC PDU combines the BPTC and CRC results.
func DecodeEmbeddedRC(embeddedData [32]bit.Bit) pdu.ReverseChannel {
	infoBits, bptcResult := bptc.DecodeSingleBurst(embeddedData, true)
	rc := pdu.DecodeReverseChannel(infoBits)
	rc.FEC = bptcResult.Combine(rc.FEC)
	return rc
}

// EncodeEmbeddedRC encodes a Reverse Channel PDU into 32 transmit
// bits suitable for the embedded signalling data field. It returns an
// error if the RC command does not fit in 4 bits.
func EncodeEmbeddedRC(rc *pdu.ReverseChannel) ([32]bit.Bit, error) {
	infoBits, err := rc.Encode()
	if err != nil {
		return [32]bit.Bit{}, err
	}
	return bptc.EncodeSingleBurst(infoBits, true), nil
}
