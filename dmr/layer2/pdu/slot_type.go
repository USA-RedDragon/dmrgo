package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/fec/golay"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
)

// ETSI TS 102 361-1 V2.5.1 (2017-10) - 9.1.3 Slot Type (SLOT) PDU
type SlotType struct {
	ColorCode int
	DataType  elements.DataType
	ParityOK  bool
}

func NewSlotTypeFromBits(data [20]byte) SlotType {
	st := SlotType{}

	for i := 0; i < 4; i++ {
		if data[i] == 1 {
			st.ColorCode |= 1 << uint(3-i)
		}
	}

	dt := 0
	for i := 4; i < 8; i++ {
		if data[i] == 1 {
			dt |= 1 << uint(7-i)
		}
	}

	st.DataType = elements.DataType(dt)

	parity := 0
	for i := 8; i < 20; i++ {
		if data[i] == 1 {
			parity |= 1 << uint(19-i)
		}
	}

	if parity < 1 {
		parity = 0
		// generate parity if not provided
		var dataBits [8]byte
		copy(dataBits[:], data[:8])
		parityBits := golay.Golay_20_8_Parity(dataBits)[8:]
		for i, v := range parityBits {
			data[8+i] = v
		}
	}

	// check parity
	st.ParityOK = golay.Golay_20_8_Check(data)

	return st
}

// ToString returns a string representation of the SlotType
func (st SlotType) ToString() string {
	return fmt.Sprintf("{ ColorCode: %d, DataType: %s, ParityOK: %t }", st.ColorCode, elements.DataTypeToName(st.DataType), st.ParityOK)
}
