package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
)

type DataHeader struct {
	dataType elements.DataType
}

func (dh *DataHeader) GetDataType() elements.DataType {
	return dh.dataType
}

func (dh *DataHeader) ToString() string {
	return fmt.Sprintf("DataHeader{ dataType: %s }", elements.DataTypeToName(dh.dataType))
}

func NewDataHeaderFromBits(infoBits [96]byte) *DataHeader {
	dh := DataHeader{}

	return &dh
}
