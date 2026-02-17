package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/bit"
	"github.com/USA-RedDragon/dmrgo/dmr/fec"
	"github.com/USA-RedDragon/dmrgo/dmr/fec/golay"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
)

// ETSI TS 102 361-1 V2.5.1 (2017-10) - 9.1.3 Slot Type (SLOT) PDU
type SlotType struct {
	ColorCode int
	DataType  elements.DataType
	FEC       fec.FECResult
}

func NewSlotTypeFromBits(data [20]bit.Bit) SlotType {
	st := SlotType{}

	corrected, result := golay.DecodeGolay2087(data)
	st.FEC = result

	if !result.Uncorrectable {
		data = corrected // Use corrected data for fields
	}

	for i := 0; i < 4; i++ {
		if data[i] == 1 {
			st.ColorCode |= 1 << (3 - i)
		}
	}

	var dt elements.DataType
	for i := 4; i < 8; i++ {
		if data[i] == 1 {
			dt |= elements.DataType(1) << (7 - i)
		}
	}

	st.DataType = dt

	return st
}

// ToString returns a string representation of the SlotType
func (st SlotType) ToString() string {
	return fmt.Sprintf("{ ColorCode: %d, DataType: %s, FEC: {BitsChecked: %d, ErrorsCorrected: %d, Uncorrectable: %t} }", st.ColorCode, elements.DataTypeToName(st.DataType), st.FEC.BitsChecked, st.FEC.ErrorsCorrected, st.FEC.Uncorrectable)
}
