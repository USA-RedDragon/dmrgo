package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
)

// ETSI TS 102 361-1 leaves the PI header content undefined (Table 6.1
// NOTE). Only the BPTC(196,96) coding and the CRC-CCITT with mask 0x6969
// (Table B.21) are specified there. The field layout below is the DMR
// Association privacy layout used by deployed equipment: algorithm ID,
// MFID, key ID, a 32-bit message indicator and the 24-bit destination.

// dmr:crc crc_ccitt
// dmr:crc_mask 0x6969
// dmr:input_size 96
// ETSI TS 102 361-1 - 6.2 PI Header (Privacy Indicator header)
type PIHeader struct {
	DataType         elements.DataType `dmr:"-"`
	crc              uint16            `dmr:"-"` //nolint:unused
	FEC              fec.FECResult     `dmr:"-"`
	AlgorithmID      uint8             `dmr:"bits:0-7"`
	MFID             uint8             `dmr:"bits:8-15"`
	KeyID            uint8             `dmr:"bits:16-23"`
	MessageIndicator uint32            `dmr:"bits:24-55"`
	Destination      int               `dmr:"bits:56-79"`
}

func (p *PIHeader) GetDataType() elements.DataType {
	return p.DataType
}
