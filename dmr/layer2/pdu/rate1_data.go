package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
)

type Rate1Data struct {
	DataType elements.DataType `dmr:"-"`
	Data     [12]byte          `dmr:"bits:0-95,packed"`
}

func (rtData *Rate1Data) GetDataType() elements.DataType {
	return rtData.DataType
}

func (rtData *Rate1Data) ToString() string {
	return fmt.Sprintf("Rate1Data{ DataType: %s, Data: %v }", elements.DataTypeToName(rtData.DataType), rtData.Data)
}
