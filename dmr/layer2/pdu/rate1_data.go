package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/bit"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
)

type Rate1Data struct {
	dataType elements.DataType
}

func (rtData *Rate1Data) GetDataType() elements.DataType {
	return rtData.dataType
}

func (rtData *Rate1Data) ToString() string {
	return fmt.Sprintf("Rate1Data{ dataType: %s }", elements.DataTypeToName(rtData.dataType))
}

func NewRate1DataFromBits(infoBits [96]bit.Bit) *Rate1Data {
	rtData := Rate1Data{}

	return &rtData
}
