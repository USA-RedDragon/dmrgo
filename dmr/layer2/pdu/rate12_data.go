package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
)

type Rate12Data struct {
	dataType elements.DataType
}

func (rtData *Rate12Data) GetDataType() elements.DataType {
	return rtData.dataType
}

func (rtData *Rate12Data) ToString() string {
	return fmt.Sprintf("Rate12Data{ dataType: %s }", elements.DataTypeToName(rtData.dataType))
}

func NewRate12DataFromBits(infoBits [96]byte) *Rate12Data {
	rtData := Rate12Data{}

	return &rtData
}
