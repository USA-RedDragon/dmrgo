package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/bit"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
)

type Rate34Data struct {
	dataType elements.DataType
}

func (rtData *Rate34Data) GetDataType() elements.DataType {
	return rtData.dataType
}

func (rtData *Rate34Data) ToString() string {
	return fmt.Sprintf("Rate34Data{ dataType: %s }", elements.DataTypeToName(rtData.dataType))
}

func NewRate34DataFromBits(infoBits [96]bit.Bit) *Rate34Data {
	rtData := Rate34Data{}

	return &rtData
}
