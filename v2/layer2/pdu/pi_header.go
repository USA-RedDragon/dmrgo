package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// PIHeader is the Privacy Indicator (PI) header, with fields in the DMRA layout (ETSI TS 102 361-1 clause 6.2).
//
//dmr:crc crc_ccitt
//dmr:crc_mask 0x6969
//dmr:input_size 96
type PIHeader struct {
	FEC              fec.Result         `dmr:"-"`          // result of the CRC check
	AlgorithmID      uint8              `dmr:"bits:0-7"`   // privacy algorithm ID
	MFID             enums.FeatureSetID `dmr:"bits:8-15"`  // manufacturer FID
	KeyID            uint8              `dmr:"bits:16-23"` // key ID
	MessageIndicator uint32             `dmr:"bits:24-55"` // 32-bit message indicator
	Destination      uint32             `dmr:"bits:56-79"` // 24-bit destination address
	// Raw holds bits 0 to 79, most significant bit first, for other layouts such as Hytera (MFID 0x68); Encode ignores it.
	Raw [10]byte `dmr:"bits:0-79,raw,no_encode"`
}

// DataType returns enums.DataTypePIHeader.
func (s *PIHeader) DataType() enums.DataType {
	return enums.DataTypePIHeader
}
