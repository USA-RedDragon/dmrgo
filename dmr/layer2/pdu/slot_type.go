package pdu

import (
	"github.com/USA-RedDragon/dmrgo/dmr/fec"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
)

// ETSI TS 102 361-1 V2.5.1 (2017-10) - 9.1.3 Slot Type (SLOT) PDU
//
//dmr:fec golay_20_8_7
type SlotType struct {
	ColorCode int               `dmr:"bits:0-3"`
	DataType  elements.DataType `dmr:"bits:4-7"`
	FEC       fec.FECResult     `dmr:"-"`
}
