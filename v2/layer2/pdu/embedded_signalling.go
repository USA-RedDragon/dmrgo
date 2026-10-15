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
	ColorCode                          enums.ColorCode `dmr:"bits:0-3"`      // Colour Code (CC)
	PreemptionAndPowerControlIndicator bool            `dmr:"bit:4"`         // Pre-emption and power control Indicator (PI)
	LCSS                               enums.LCSS      `dmr:"bits:5-6,enum"` // LC Start/Stop (LCSS) of the embedded signalling
	FEC                                fec.Result      `dmr:"-"`             // QR(16,7,6) result
}
