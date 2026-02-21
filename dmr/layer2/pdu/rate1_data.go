package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
)

// ETSI TS 102 361-1 V2.5.1 (2017-10) - 9.1.7 Rate 1 data
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
