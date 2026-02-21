package pdu

import (
	"fmt"

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

// ToString returns a string representation of the SlotType
func (st SlotType) ToString() string {
	return fmt.Sprintf("{ ColorCode: %d, DataType: %s, FEC: {BitsChecked: %d, ErrorsCorrected: %d, Uncorrectable: %t} }", st.ColorCode, elements.DataTypeToName(st.DataType), st.FEC.BitsChecked, st.FEC.ErrorsCorrected, st.FEC.Uncorrectable)
}
