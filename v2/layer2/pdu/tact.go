package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// ETSI TS 102 361-1 - 9.1.4 TACT PDU
//
// The TACT (Timing, Activity, Channel, Type) PDU is a 7-bit field
// at the start of each CACH burst. It carries 4 information bits
// protected by Hamming(7,4,3) FEC.
//
// Layout: [AT, TC, LCSS₀, LCSS₁, p₀, p₁, p₂]
//
//dmr:fec hamming_7_4_3
type TACT struct {
	AccessType  bool          `dmr:"bit:0"`
	TDMAChannel bool          `dmr:"bit:1"`
	LCSS        enums.LCSS    `dmr:"bits:2-3,enum"`
	FEC         fec.FECResult `dmr:"-"`
}
