package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
)

// ETSI TS 102 361-1 - 9.1.7 Rate 3/4 data
type Rate34Data struct {
	DataType elements.DataType `dmr:"-"`
	Data     [12]byte          `dmr:"bits:0-95,packed"`
}

func (rtData *Rate34Data) GetDataType() elements.DataType {
	return rtData.DataType
}
