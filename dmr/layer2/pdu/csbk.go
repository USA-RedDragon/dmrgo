package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
)

type CSBK struct {
	dataType elements.DataType
}

func (csbk *CSBK) GetDataType() elements.DataType {
	return csbk.dataType
}

func (csbk *CSBK) ToString() string {
	return fmt.Sprintf("CSBK{ dataType: %s }", elements.DataTypeToName(csbk.dataType))
}

func NewCSBKFromBits(infoBits [96]byte) *CSBK {
	csbk := CSBK{}

	return &csbk
}
