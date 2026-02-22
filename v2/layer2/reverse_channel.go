package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// ETSI TS 102 361-1 §6.4 — Reverse Channel
//
// The Reverse Channel (RC) provides inbound signalling from an MS to a BS
// during an active voice call. RC data is carried in the 32-bit embedded
// signalling data field of a voice burst.
//
// Two forms:
//   - §6.4.1 Standalone inbound RC: MS sends burst with MsSourcedRcSync
//     pattern. The 48-bit center contains EMB (16 bits) + RC data (32 bits).
//   - §6.4.2 Outbound embedded RC: BS echoes RC in a normal voice burst
//     with EMB PI=1 and LCSS=SingleFragmentLCorCSBK.
//
// Detection: a voice burst's embedded signalling has
// PreemptionAndPowerControlIndicator=true (PI=1) AND
// LCSS=SingleFragmentLCorCSBK (0) → the 32-bit data is RC, not LC.
//
// Processing: 32 transmit bits → Single Burst BPTC (oddParity=true) →
// 11 info bits → RC PDU decode (4 payload + 7 CRC-7 with mask 0x7A).

// DecodeRCFromEmbeddedData decodes a Reverse Channel PDU from the 32-bit
// embedded signalling data field. It applies Single Burst Variable Length
// BPTC with odd parity (§B.2.2) followed by CRC-7 verification (§B.3.13).
//
// Returns the decoded RC PDU and a combined FEC result covering both the
// BPTC and CRC layers.
func DecodeRCFromEmbeddedData(embeddedData [32]bit.Bit) (pdu.ReverseChannel, fec.FECResult) {
	// Single Burst BPTC decode with odd parity (RC mode)
	infoBits, bptcResult := bptc.DecodeSingleBurstBPTC(embeddedData, true)

	// Decode RC PDU (CRC-7 check)
	rc, crcResult := pdu.DecodeReverseChannel(infoBits)

	// Combine FEC results
	combined := fec.FECResult{
		BitsChecked:     bptcResult.BitsChecked + crcResult.BitsChecked,
		ErrorsCorrected: bptcResult.ErrorsCorrected + crcResult.ErrorsCorrected,
		Uncorrectable:   bptcResult.Uncorrectable || crcResult.Uncorrectable,
	}
	rc.FEC = combined

	return rc, combined
}

// EncodeRCToEmbeddedData encodes a Reverse Channel PDU into 32 transmit
// bits suitable for the embedded signalling data field.
func EncodeRCToEmbeddedData(rc *pdu.ReverseChannel) [32]bit.Bit {
	infoBits := pdu.EncodeReverseChannel(rc)
	return bptc.EncodeSingleBurstBPTC(infoBits, true)
}
