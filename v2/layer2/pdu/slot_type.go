package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// ETSI TS 102 361-1 - 9.1.3 Slot Type (SLOT) PDU
//
//dmr:fec golay_20_8_7
type SlotType struct {
	ColorCode int            `dmr:"bits:0-3"`
	DataType  enums.DataType `dmr:"bits:4-7"`
	FEC       fec.Result     `dmr:"-"`
}
