package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// PIHeader is the Privacy Indicator (PI) header. ETSI TS 102 361-1 defines
// only its burst: Data Type 0000 (clause 9.3.6), BPTC(196,96) with a
// CRC-CCITT and the mask 0x6969 (Annex B), sent after the voice LC header
// (clause 5.1.2.2). The layout and meaning of the fields follow common
// practice; no public ETSI text defines them.
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
	if s == nil {
		return 0
	}
	return enums.DataTypePIHeader
}
