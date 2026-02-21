package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/enums"
	"github.com/USA-RedDragon/dmrgo/dmr/fec"
)

// ETSI TS 102 361-1 V2.5.1 (2017-10) - 9.1.2 Embedded signalling (EMB) PDU
//
//dmr:fec quadratic_residue_16_7_6
type EmbeddedSignalling struct {
	ColorCode                          int           `dmr:"bits:0-3"`
	PreemptionAndPowerControlIndicator bool          `dmr:"bit:4"`
	LCSS                               enums.LCSS    `dmr:"bits:5-6,enum"`
	FEC                                fec.FECResult `dmr:"-"`
}

// ToString returns a string representation of the EmbeddedSignalling
func (es EmbeddedSignalling) ToString() string {
	return fmt.Sprintf("{ Color Code: %d, Preemption and Power Control Indicator: %t, LCSS: %s, FEC: {BitsChecked: %d, ErrorsCorrected: %d, Uncorrectable: %t} }", es.ColorCode, es.PreemptionAndPowerControlIndicator, enums.LCSSToName(es.LCSS), es.FEC.BitsChecked, es.FEC.ErrorsCorrected, es.FEC.Uncorrectable)
}
