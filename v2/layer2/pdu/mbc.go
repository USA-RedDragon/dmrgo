package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	bits "github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// MBCHeader is the header block of a Multi Block Control (MBC) PDU (ETSI TS 102 361-1 clause 7.4.1).
//
//dmr:crc crc_ccitt
//dmr:crc_mask 0xAAAA
//dmr:input_size 96
//dmr:dispatch_if FID==0
type MBCHeader struct {
	FEC         fec.Result         `dmr:"-"`         // result of the CRC check
	LastBlock   bool               `dmr:"bit:0"`     // LB bit, false in an MBC header
	ProtectFlag bool               `dmr:"bit:1"`     // PF bit
	Opcode      CSBKOpcode         `dmr:"bits:2-7"`  // CSBKO, selects the payload field
	FID         enums.FeatureSetID `dmr:"bits:8-15"` // feature set ID, only the standard FID has payload fields

	PrivateVoiceGrantPDU            *PrivateVoiceGrantPDU            `dmr:"bits:16-79,dispatch:Opcode=CSBKPrivateVoiceGrant"`            // PV_GRANT payload, or nil
	TalkgroupVoiceGrantPDU          *TalkgroupVoiceGrantPDU          `dmr:"bits:16-79,dispatch:Opcode=CSBKTalkgroupVoiceGrant"`          // TV_GRANT payload, or nil
	BroadcastTalkgroupVoiceGrantPDU *BroadcastTalkgroupVoiceGrantPDU `dmr:"bits:16-79,dispatch:Opcode=CSBKBroadcastTalkgroupVoiceGrant"` // BTV_GRANT payload, or nil
	PrivateDataGrantPDU             *PrivateDataGrantPDU             `dmr:"bits:16-79,dispatch:Opcode=CSBKPrivateDataGrant"`             // PD_GRANT payload, or nil
	TalkgroupDataGrantPDU           *TalkgroupDataGrantPDU           `dmr:"bits:16-79,dispatch:Opcode=CSBKTalkgroupDataGrant"`           // TD_GRANT payload, or nil
	DuplexPrivateVoiceGrantPDU      *DuplexPrivateVoiceGrantPDU      `dmr:"bits:16-79,dispatch:Opcode=CSBKDuplexPrivateVoiceGrant"`      // PV_GRANT_DX payload, or nil
	DuplexPrivateDataGrantPDU       *DuplexPrivateDataGrantPDU       `dmr:"bits:16-79,dispatch:Opcode=CSBKDuplexPrivateDataGrant"`       // PD_GRANT_DX payload, or nil
	PrivateDataGrantMultiItemPDU    *PrivateDataGrantMultiItemPDU    `dmr:"bits:16-79,dispatch:Opcode=CSBKPrivateDataGrantMultiItem"`    // PD_GRANT_MI payload, or nil
	MovePDU                         *MovePDU                         `dmr:"bits:16-79,dispatch:Opcode=CSBKMove"`                         // C_MOVE payload, or nil
	BroadcastPDU                    *BroadcastPDU                    `dmr:"bits:16-79,dispatch:Opcode=CSBKBroadcast"`                    // C_BCAST payload, or nil

	// RawPayload holds header octets 2 to 9 (bits 16 to 79) when no payload
	// field is set.
	RawPayload [8]byte `dmr:"bits:16-79,raw,fallback"`
}

// DataType returns enums.DataTypeMBCHeader.
func (s *MBCHeader) DataType() enums.DataType {
	if s == nil {
		return 0
	}
	return enums.DataTypeMBCHeader
}

// MBCContinuation is a continuation block of a Multi Block Control (MBC) PDU.
// DecodeMBCContinuation does not check the last-block CRC: check it with
// MBCLastBlockCRCOK over all continuation blocks.
// (ETSI TS 102 361-1 clause 7.4.1)
type MBCContinuation struct {
	LastBlock bool `dmr:"bit:0"` // LB bit, true for the last block, which holds the CRC

	// Data holds bits 1 to 95 of the block, packed most significant bit
	// first. A last block holds the CRC in bits 80 to 95. The low bit of
	// byte 11 is padding: Encode returns a *dmrgo.FieldRangeError when it is set.
	Data [12]byte `dmr:"bits:1-95,raw"`
}

// DataType returns enums.DataTypeMBCContinuation.
func (s *MBCContinuation) DataType() enums.DataType {
	if s == nil {
		return 0
	}
	return enums.DataTypeMBCContinuation
}

func (s *MBCContinuation) packed() [12]byte {
	var data [12]byte
	if s.LastBlock {
		data[0] = 0x80
	}
	bits.Copy(data[:], 1, s.Data[:], 0, 95)
	return data
}

// AsCGAP decodes the block as the CG_AP continuation of a grant MBC. The FEC
// field of the result reports the last-block CRC, which applies only when
// this is the one continuation block of the MBC.
// (ETSI TS 102 361-4 Table 7.16)
func (s *MBCContinuation) AsCGAP() CGAPContinuation {
	if s == nil {
		return CGAPContinuation{}
	}
	return decodeCGAPContinuation(s.packed())
}

// AsMVAP decodes the block as the MV_AP continuation of a C_MOVE MBC (ETSI TS
// 102 361-4 Table 7.18), as AsCGAP does.
func (s *MBCContinuation) AsMVAP() MVAPContinuation {
	if s == nil {
		return MVAPContinuation{}
	}
	return decodeMVAPContinuation(s.packed())
}

// AsBCAP decodes the block as the BC_AP continuation of a C_BCAST MBC (ETSI
// TS 102 361-4 Table 7.21), as AsCGAP does.
func (s *MBCContinuation) AsBCAP() BCAPContinuation {
	if s == nil {
		return BCAPContinuation{}
	}
	return decodeBCAPContinuation(s.packed())
}

// AsVNAP decodes the block as the VN_AP continuation of a C_BCAST Vote_Now
// MBC (ETSI TS 102 361-4 Table 7.72), as AsCGAP does.
func (s *MBCContinuation) AsVNAP() VNAPContinuation {
	if s == nil {
		return VNAPContinuation{}
	}
	return decodeVNAPContinuation(s.packed())
}

func mbcLastBlockCRC(blocks ...[12]byte) uint16 {
	if len(blocks) == 0 {
		return 0
	}
	var reg uint16
	for i := range blocks {
		n := 12
		if i == len(blocks)-1 {
			n = 10
		}
		reg = crc.CCITTUpdate(reg, blocks[i][:n])
	}
	return ^reg
}

// MBCLastBlockCRCOK reports whether bits 80 to 95 of the last of blocks hold
// the last-block CRC-CCITT that SetMBCLastBlockCRC writes. Each block holds
// its 96 bits packed, the first bit in the most significant bit of byte 0.
// It returns false for no blocks.
func MBCLastBlockCRCOK(blocks ...[12]byte) bool {
	if len(blocks) == 0 {
		return false
	}
	last := blocks[len(blocks)-1]
	return uint16(last[10])<<8|uint16(last[11]) == mbcLastBlockCRC(blocks...)
}

// SetMBCLastBlockCRC writes the last-block CRC-CCITT into bits 80 to 95 of
// the last of blocks, which holds every continuation block of the MBC in
// order, packed as MBCLastBlockCRCOK takes them. It does nothing for no
// blocks. (ETSI TS 102 361-1 clause 7.4.0)
func SetMBCLastBlockCRC(blocks [][12]byte) {
	if len(blocks) == 0 {
		return
	}
	c := mbcLastBlockCRC(blocks...)
	last := &blocks[len(blocks)-1]
	last[10], last[11] = byte(c>>8), byte(c&0xFF)
}

// CGAPContinuation is the CG_AP continuation block of a grant MBC (ETSI TS 102 361-4 clause 7.1.1.1.2, Table 7.16).
//
//dmr:crc crc_ccitt
//dmr:input_size 96
//dmr:subpdu
type CGAPContinuation struct {
	FEC         fec.Result       `dmr:"-"`                   // result of the last-block CRC check
	LastBlock   bool             `dmr:"bit:0"`               // LB bit, true for the last block
	ProtectFlag bool             `dmr:"bit:1"`               // PF bit
	Opcode      CSBKOpcode       `dmr:"bits:2-7"`            // CSBKO of the MBC header
	Reserved1   uint8            `dmr:"bits:8-11"`           // reserved, 0
	ColorCode   enums.ColorCode  `dmr:"bits:12-15"`          // color code of the destination channel
	Cdeftype    uint8            `dmr:"bits:16-19"`          // type of CdefParms, 0 for absolute frequencies
	Reserved2   uint8            `dmr:"bits:20-21"`          // reserved, 0
	CdefParms   layer3.CdefParms `dmr:"bits:22-79,delegate"` // channel number and its TX and RX frequencies
}

// DataType returns enums.DataTypeMBCContinuation.
func (s *CGAPContinuation) DataType() enums.DataType {
	if s == nil {
		return 0
	}
	return enums.DataTypeMBCContinuation
}

// MVAPContinuation is the MV_AP continuation block of a C_MOVE MBC (ETSI TS 102 361-4 clause 7.1.1.1.3.1, Table 7.18).
//
//dmr:crc crc_ccitt
//dmr:input_size 96
//dmr:subpdu
type MVAPContinuation struct {
	FEC         fec.Result       `dmr:"-"`                   // result of the last-block CRC check
	LastBlock   bool             `dmr:"bit:0"`               // LB bit, true for the last block
	ProtectFlag bool             `dmr:"bit:1"`               // PF bit
	Opcode      CSBKOpcode       `dmr:"bits:2-7"`            // CSBKO of the MBC header
	Reserved1   uint8            `dmr:"bits:8-11"`           // reserved, 0
	ColorCode   enums.ColorCode  `dmr:"bits:12-15"`          // color code of the destination channel
	Cdeftype    uint8            `dmr:"bits:16-19"`          // type of CdefParms, 0 for absolute frequencies
	Reserved2   uint8            `dmr:"bits:20-21"`          // reserved, 0
	CdefParms   layer3.CdefParms `dmr:"bits:22-79,delegate"` // channel number and its TX and RX frequencies
}

// DataType returns enums.DataTypeMBCContinuation.
func (s *MVAPContinuation) DataType() enums.DataType {
	if s == nil {
		return 0
	}
	return enums.DataTypeMBCContinuation
}

// BCAPContinuation is the BC_AP continuation block of a C_BCAST MBC (ETSI TS 102 361-4 clause 7.1.1.1.5.1, Table 7.21).
//
//dmr:crc crc_ccitt
//dmr:input_size 96
//dmr:subpdu
type BCAPContinuation struct {
	FEC         fec.Result       `dmr:"-"`                   // result of the last-block CRC check
	LastBlock   bool             `dmr:"bit:0"`               // LB bit, true for the last block
	ProtectFlag bool             `dmr:"bit:1"`               // PF bit
	Opcode      CSBKOpcode       `dmr:"bits:2-7"`            // CSBKO of the MBC header
	Reserved1   byte             `dmr:"bits:8-15"`           // reserved, 0
	Cdeftype    uint8            `dmr:"bits:16-19"`          // type of CdefParms, 0 for absolute frequencies
	Reserved2   uint8            `dmr:"bits:20-21"`          // reserved, 0
	CdefParms   layer3.CdefParms `dmr:"bits:22-79,delegate"` // channel number and its TX and RX frequencies
}

// DataType returns enums.DataTypeMBCContinuation.
func (s *BCAPContinuation) DataType() enums.DataType {
	if s == nil {
		return 0
	}
	return enums.DataTypeMBCContinuation
}

// VNAPContinuation is the VN_AP continuation block of a C_BCAST Vote_Now MBC (ETSI TS 102 361-4 clause 7.2.19.3.1, Table 7.72).
//
//dmr:crc crc_ccitt
//dmr:input_size 96
//dmr:subpdu
type VNAPContinuation struct {
	FEC         fec.Result       `dmr:"-"`                   // result of the last-block CRC check
	LastBlock   bool             `dmr:"bit:0"`               // LB bit, true for the last block
	ProtectFlag bool             `dmr:"bit:1"`               // PF bit
	Opcode      CSBKOpcode       `dmr:"bits:2-7"`            // CSBKO of the MBC header
	Reserved1   uint8            `dmr:"bits:8-11"`           // reserved, 0
	ColorCode   enums.ColorCode  `dmr:"bits:12-15"`          // color code of the destination channel
	Cdeftype    uint8            `dmr:"bits:16-19"`          // type of CdefParms, 0 for absolute frequencies
	Reserved2   uint8            `dmr:"bits:20-21"`          // reserved, 0
	CdefParms   layer3.CdefParms `dmr:"bits:22-79,delegate"` // channel number and its TX and RX frequencies
}

// DataType returns enums.DataTypeMBCContinuation.
func (s *VNAPContinuation) DataType() enums.DataType {
	if s == nil {
		return 0
	}
	return enums.DataTypeMBCContinuation
}
