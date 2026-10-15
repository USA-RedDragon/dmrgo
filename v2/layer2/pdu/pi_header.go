package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// ETSI TS 102 361-1 - 6.2 PI Header (Privacy Indicator header)
//
// ETSI TS 102 361-1 leaves the PI header content undefined (Table 6.1
// NOTE). Only the BPTC(196,96) coding and the CRC-CCITT with mask 0x6969
// (Table B.21) are specified there. The named fields use the DMR
// Association (DMRA) privacy layout used by Motorola and others: algorithm
// ID, MFID, key ID, a 32-bit message indicator and the 24-bit destination.
// Other vendors, such as Hytera (MFID 0x68), use other layouts, so Raw
// holds all 80 bits before the CRC for callers to decode themselves.
// PIHeader.Encode writes the named fields and ignores Raw.
//
//dmr:crc crc_ccitt
//dmr:crc_mask 0x6969
//dmr:input_size 96
type PIHeader struct {
	FEC              fec.Result         `dmr:"-"`
	AlgorithmID      uint8              `dmr:"bits:0-7"`
	MFID             enums.FeatureSetID `dmr:"bits:8-15"`
	KeyID            uint8              `dmr:"bits:16-23"`
	MessageIndicator uint32             `dmr:"bits:24-55"`
	Destination      uint32             `dmr:"bits:56-79"`
	// Raw holds bits 0 to 79, packed most significant bit first: bit 0 is
	// the top bit of byte 0.
	Raw [10]byte `dmr:"bits:0-79,raw,no_encode"`
}

// DataType returns enums.DataTypePIHeader.
func (s *PIHeader) DataType() enums.DataType {
	return enums.DataTypePIHeader
}
