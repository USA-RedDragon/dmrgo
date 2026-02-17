package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/bit"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
)

type Rate12Data struct {
	dataType elements.DataType
	Data     [12]byte
}

func (rtData *Rate12Data) GetDataType() elements.DataType {
	return rtData.dataType
}

func (rtData *Rate12Data) ToString() string {
	return fmt.Sprintf("Rate12Data{ dataType: %s, Data: %v }", elements.DataTypeToName(rtData.dataType), rtData.Data)
}

func (rtData *Rate12Data) DecodeFromBits(infoBits []bit.Bit, dt elements.DataType) bool {
	rtData.dataType = dt

	if len(infoBits) != 96 {
		fmt.Println("Rate12Data: invalid infoBits length: ", len(infoBits))
		return false
	}

	for i := 0; i < 12; i++ {
		for j := 0; j < 8; j++ {
			if infoBits[i*8+j] == 1 {
				rtData.Data[i] |= 1 << (7 - j)
			}
		}
	}

	return true
}
