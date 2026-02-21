package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
)

type Rate12Data struct {
	DataType elements.DataType `dmr:"-"`
	Data     [12]byte          `dmr:"bits:0-95,packed"`
}

func (rtData *Rate12Data) GetDataType() elements.DataType {
	return rtData.DataType
}

func (rtData *Rate12Data) ToString() string {
	return fmt.Sprintf("Rate12Data{ DataType: %s, Data: %v }", elements.DataTypeToName(rtData.DataType), rtData.Data)
}
