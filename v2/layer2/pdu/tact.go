package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// TACT is the TDMA Access Channel Type PDU. Each CACH burst carries one: AT,
// TC and LCSS, then 3 Hamming(7,4,3) parity bits.
// (ETSI TS 102 361-1 clause 9.1.4)
//
//dmr:fec hamming_7_4_3
type TACT struct {
	AccessType  bool       `dmr:"bit:0"`         // Access Type (AT): true when the inbound channel is busy
	TDMAChannel bool       `dmr:"bit:1"`         // TDMA Channel (TC): true when the next outbound burst is channel 2
	LCSS        enums.LCSS `dmr:"bits:2-3,enum"` // LC Start/Stop (LCSS) of the CACH signalling
	FEC         fec.Result `dmr:"-"`             // Hamming(7,4,3) result
}
