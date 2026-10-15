package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// SlotType is the slot type (SLOT) PDU (ETSI TS 102 361-1 clause 9.1.3).
//
//dmr:fec golay_20_8_7
type SlotType struct {
	ColorCode enums.ColorCode `dmr:"bits:0-3"`
	DataType  enums.DataType  `dmr:"bits:4-7,enum"`
	FEC       fec.Result      `dmr:"-"`
}
