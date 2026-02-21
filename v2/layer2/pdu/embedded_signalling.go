package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// ETSI TS 102 361-1 - 9.1.2 Embedded signalling (EMB) PDU
//
//dmr:fec quadratic_residue_16_7_6
type EmbeddedSignalling struct {
	ColorCode                          int           `dmr:"bits:0-3"`
	PreemptionAndPowerControlIndicator bool          `dmr:"bit:4"`
	LCSS                               enums.LCSS    `dmr:"bits:5-6,enum"`
	FEC                                fec.FECResult `dmr:"-"`
}
