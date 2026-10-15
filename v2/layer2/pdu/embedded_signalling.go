package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// EmbeddedSignalling is the embedded signalling (EMB) PDU (ETSI TS 102 361-1
// clause 9.1.2).
//
//dmr:fec quadratic_residue_16_7_6
type EmbeddedSignalling struct {
	ColorCode                          enums.ColorCode `dmr:"bits:0-3"`
	PreemptionAndPowerControlIndicator bool            `dmr:"bit:4"`
	LCSS                               enums.LCSS      `dmr:"bits:5-6,enum"`
	FEC                                fec.Result      `dmr:"-"`
}
