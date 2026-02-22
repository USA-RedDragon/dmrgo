package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
)

// ETSI TS 102 361-2 - 7.1.1 Short LC Null Message PDU
type ShortLCNullMessage struct {
	Reserved [24]bit.Bit `dmr:"bits:0-23,raw"`
}

// ETSI TS 102 361-2 - 7.1.2 Activity Update PDU
type ShortLCActivityUpdate struct {
	TS1ActivityID enums.ActivityID `dmr:"bits:0-3,enum"`
	TS2ActivityID enums.ActivityID `dmr:"bits:4-7,enum"`
	HashTS1       byte             `dmr:"bits:8-15"`
	HashTS2       byte             `dmr:"bits:16-23"`
}

// dmr:crc crc_8
// dmr:input_size 36
// ETSI TS 102 361-1 - 9.1.7 Short Link Control (SHORT_LC) PDU
type ShortLC struct {
	DataType elements.DataType `dmr:"-"`

	SLCO enums.SLCO    `dmr:"bits:0-3,enum"`
	FEC  fec.FECResult `dmr:"-"`

	NullMessage    *ShortLCNullMessage    `dmr:"bits:4-27,dispatch:SLCO=enums.SLCONullMessage"`
	ActivityUpdate *ShortLCActivityUpdate `dmr:"bits:4-27,dispatch:SLCO=enums.SLCOActivityUpdate"`

	crc uint8 `dmr:"-"` //nolint:unused
}

func (slc *ShortLC) GetDataType() elements.DataType {
	return slc.DataType
}
