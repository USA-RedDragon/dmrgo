package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/bit"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
)

type PIHeader struct {
	dataType elements.DataType
}

func (ph *PIHeader) GetDataType() elements.DataType {
	return ph.dataType
}

func (ph *PIHeader) ToString() string {
	return fmt.Sprintf("PIHeader{ dataType: %s }", elements.DataTypeToName(ph.dataType))
}

func NewPIHeaderFromBits(infoBits [96]bit.Bit) *PIHeader {
	ph := PIHeader{}

	return &ph
}
