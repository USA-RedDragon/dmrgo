package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// ETSI TS 102 361-1 - 7.4.1 Multi Block Control (MBC) Header
//
// The header of a Tier III grant, C_MOVE or C_BCAST MBC carries the CSBK
// payload of its opcode (ETSI TS 102 361-4 §7.1.1.1), so with the standard
// FID DecodeMBCHeader sets the payload field of the opcode, as DecodeCSBK
// does. Payloads of other opcodes and FIDs, and of opcode 0x38, are kept in
// RawPayload. Encode returns an error if more than one payload field is set,
// or if Opcode and FID do not select the payload field that is set.
//
//dmr:crc crc_ccitt
//dmr:crc_mask 0xAAAA
//dmr:input_size 96
//dmr:dispatch_if FID==0
type MBCHeader struct {
	FEC         fec.Result         `dmr:"-"`
	LastBlock   bool               `dmr:"bit:0"`
	ProtectFlag bool               `dmr:"bit:1"`
	Opcode      CSBKOpcode         `dmr:"bits:2-7"`
	FID         enums.FeatureSetID `dmr:"bits:8-15"`

	PrivateVoiceGrantPDU            *PrivateVoiceGrantPDU            `dmr:"bits:16-79,dispatch:Opcode=CSBKPrivateVoiceGrant"`
	TalkgroupVoiceGrantPDU          *TalkgroupVoiceGrantPDU          `dmr:"bits:16-79,dispatch:Opcode=CSBKTalkgroupVoiceGrant"`
	BroadcastTalkgroupVoiceGrantPDU *BroadcastTalkgroupVoiceGrantPDU `dmr:"bits:16-79,dispatch:Opcode=CSBKBroadcastTalkgroupVoiceGrant"`
	PrivateDataGrantPDU             *PrivateDataGrantPDU             `dmr:"bits:16-79,dispatch:Opcode=CSBKPrivateDataGrant"`
	TalkgroupDataGrantPDU           *TalkgroupDataGrantPDU           `dmr:"bits:16-79,dispatch:Opcode=CSBKTalkgroupDataGrant"`
	DuplexPrivateVoiceGrantPDU      *DuplexPrivateVoiceGrantPDU      `dmr:"bits:16-79,dispatch:Opcode=CSBKDuplexPrivateVoiceGrant"`
	DuplexPrivateDataGrantPDU       *DuplexPrivateDataGrantPDU       `dmr:"bits:16-79,dispatch:Opcode=CSBKDuplexPrivateDataGrant"`
	PrivateDataGrantMultiItemPDU    *PrivateDataGrantMultiItemPDU    `dmr:"bits:16-79,dispatch:Opcode=CSBKPrivateDataGrantMultiItem"`
	MovePDU                         *MovePDU                         `dmr:"bits:16-79,dispatch:Opcode=CSBKMove"`
	BroadcastPDU                    *BroadcastPDU                    `dmr:"bits:16-79,dispatch:Opcode=CSBKBroadcast"`

	// RawPayload holds header bits 16 to 79 when no payload field is set,
	// packed most significant bit first, as CSBK.RawPayload does.
	RawPayload [8]byte `dmr:"bits:16-79,raw,fallback"`
}

// DataType returns enums.DataTypeMBCHeader.
func (s *MBCHeader) DataType() enums.DataType {
	return enums.DataTypeMBCHeader
}

// ETSI TS 102 361-1 - 7.4.1 Multi Block Control (MBC) Continuation
//
// The continuation block carries either intermediate or last-block data.
// LB=0: intermediate block (95 data bits, no CRC)
// LB=1: last block (79 data bits + 16-bit CRC)
//
// The last-block CRC-CCITT covers every MBC block except the header (§7.4.0),
// so a single block can only be checked when it is the only continuation
// block. Use mbcLastBlockCRC, CheckMBCLastBlockCRC and SetMBCLastBlockCRC with
// all continuation blocks in order. It has no Data Type mask (Table B.21
// NOTE 1).
type MBCContinuation struct {
	// LastBlock indicates whether this is the final continuation block.
	// LB=1 means last block (has CRC), LB=0 means intermediate (no CRC).
	LastBlock bool `dmr:"bit:0"`

	// Data holds bits 1 to 95 of the block, packed most significant bit
	// first: bit 1 is the top bit of byte 0 and bit 95 is bit 1 of byte
	// 11. The low bit of byte 11 is padding and must be zero; Encode
	// returns a *bit.FieldRangeError otherwise. An intermediate block
	// (LB=0) carries 95 data bits; a last block (LB=1) carries 79 data
	// bits, bits 1 to 79, and the CRC in bits 80 to 95.
	Data [12]byte `dmr:"bits:1-95,raw"`
}

// DataType returns enums.DataTypeMBCContinuation.
func (s *MBCContinuation) DataType() enums.DataType {
	return enums.DataTypeMBCContinuation
}

func (s *MBCContinuation) bits() [96]bit.Bit {
	var data [96]bit.Bit
	if s.LastBlock {
		data[0] = 1
	}
	copy(data[1:], bit.Unpack(s.Data[:]))
	return data
}

// AsCGAP decodes the block as the CGAP continuation of a grant MBC (ETSI TS
// 102 361-4 Table 7.16). DecodeMBCContinuation decodes every continuation
// block alike because its layout depends on the MBC header, so the caller
// picks the layout from MBCHeader.Opcode. The FEC field of the result
// reports the last-block CRC, which applies only when this is the one
// continuation block of the MBC.
func (s *MBCContinuation) AsCGAP() CGAPContinuation {
	return decodeCGAPContinuation(s.bits())
}

// AsMVAP decodes the block as the MVAP continuation of a C_MOVE MBC (ETSI TS
// 102 361-4 Table 7.18), as AsCGAP does.
func (s *MBCContinuation) AsMVAP() MVAPContinuation {
	return decodeMVAPContinuation(s.bits())
}

// AsBCAP decodes the block as the BCAP continuation of a C_BCAST MBC (ETSI
// TS 102 361-4 Table 7.21), as AsCGAP does.
func (s *MBCContinuation) AsBCAP() BCAPContinuation {
	return decodeBCAPContinuation(s.bits())
}

// AsVNAP decodes the block as the VNAP continuation of a C_BCAST Vote_Now
// MBC (ETSI TS 102 361-4 Table 7.72), as AsCGAP does.
func (s *MBCContinuation) AsVNAP() VNAPContinuation {
	return decodeVNAPContinuation(s.bits())
}

// mbcLastBlockCRC computes the last-block CRC-CCITT of blocks.
func mbcLastBlockCRC(blocks ...[96]bit.Bit) uint16 {
	if len(blocks) == 0 {
		return 0
	}
	data := make([]byte, 0, len(blocks)*12)
	for i, b := range blocks {
		n := 96
		if i == len(blocks)-1 {
			n = 80
		}
		data = append(data, bit.Pack(b[:n])...)
	}
	return crc.CCITT(data)
}

// MBCLastBlockCRCOK reports whether bits 80 to 95 of the last of blocks hold
// the last-block CRC-CCITT that SetMBCLastBlockCRC writes. It returns false
// for no blocks.
func MBCLastBlockCRCOK(blocks ...[96]bit.Bit) bool {
	if len(blocks) == 0 {
		return false
	}
	last := blocks[len(blocks)-1]
	return uint16(bit.Uint(last[80:96])) == mbcLastBlockCRC(blocks...) //nolint:gosec // 16-bit field
}

// SetMBCLastBlockCRC writes the last-block CRC-CCITT (ETSI TS 102 361-1
// §7.4.0) into bits 80 to 95 of the last of blocks. The CRC covers all 96
// bits of each block before it and bits 0 to 79 of the last block, so
// blocks holds every continuation block of the MBC in order, last block
// last. It does nothing for no blocks.
func SetMBCLastBlockCRC(blocks [][96]bit.Bit) {
	if len(blocks) == 0 {
		return
	}
	c := mbcLastBlockCRC(blocks...)
	last := &blocks[len(blocks)-1]
	for i := 0; i < 16; i++ {
		last[80+i] = bit.Bit((c >> (15 - i)) & 1)
	}
}

// ── Tier III MBC Continuation Types ──
// These structs parse the 96-bit BPTC output of a continuation block
// based on the MBC header's CSBKO. They are always last-blocks (LB=1)
// that directly follow the header, so the generated decoder checks the
// last-block CRC over bits 0-79 and the encoder computes it.
//
// Usage: after receiving an MBCContinuation with LB=1, the higher layer
// determines the type from the preceding MBCHeader.Opcode and
// calls the appropriate Decode function on the full 96-bit block.

// CGAPContinuation represents Channel Grant Absolute Parameters.
// Used with all grant opcodes (PV_GRANT, TV_GRANT, BTV_GRANT,
// PD_GRANT, TD_GRANT, PV_GRANT_DX, PD_GRANT_DX, PD_GRANT_MI,
// TD_GRANT_MI) when PhysicalChannel=0xFFF.
// ETSI TS 102 361-4 — §7.1.1.1.2, Table 7.16
//
//dmr:crc crc_ccitt
//dmr:input_size 96
//dmr:subpdu
type CGAPContinuation struct {
	FEC         fec.Result       `dmr:"-"`
	LastBlock   bool             `dmr:"bit:0"`
	ProtectFlag bool             `dmr:"bit:1"`
	Opcode      CSBKOpcode       `dmr:"bits:2-7"`
	Reserved1   uint8            `dmr:"bits:8-11"`
	ColorCode   enums.ColorCode  `dmr:"bits:12-15"`
	Cdeftype    uint8            `dmr:"bits:16-19"`
	Reserved2   uint8            `dmr:"bits:20-21"`
	CdefParms   layer3.CdefParms `dmr:"bits:22-79,delegate"`
}

// DataType returns enums.DataTypeMBCContinuation.
func (s *CGAPContinuation) DataType() enums.DataType {
	return enums.DataTypeMBCContinuation
}

// MVAPContinuation represents Move Absolute Parameters.
// Used with C_MOVE (opcode 0x39) when PhysicalChannel=0xFFF.
// ETSI TS 102 361-4 — §7.1.1.1.3.1, Table 7.18
//
//dmr:crc crc_ccitt
//dmr:input_size 96
//dmr:subpdu
type MVAPContinuation struct {
	FEC         fec.Result       `dmr:"-"`
	LastBlock   bool             `dmr:"bit:0"`
	ProtectFlag bool             `dmr:"bit:1"`
	Opcode      CSBKOpcode       `dmr:"bits:2-7"`
	Reserved1   uint8            `dmr:"bits:8-11"`
	ColorCode   enums.ColorCode  `dmr:"bits:12-15"`
	Cdeftype    uint8            `dmr:"bits:16-19"`
	Reserved2   uint8            `dmr:"bits:20-21"`
	CdefParms   layer3.CdefParms `dmr:"bits:22-79,delegate"`
}

// DataType returns enums.DataTypeMBCContinuation.
func (s *MVAPContinuation) DataType() enums.DataType {
	return enums.DataTypeMBCContinuation
}

// BCAPContinuation represents Broadcast Absolute Parameters.
// Used with C_BCAST (opcode 0x28) for Ann_WD_TSCC and Chan_Freq.
// ETSI TS 102 361-4 — §7.1.1.1.5.1, Table 7.21
//
//dmr:crc crc_ccitt
//dmr:input_size 96
//dmr:subpdu
type BCAPContinuation struct {
	FEC         fec.Result       `dmr:"-"`
	LastBlock   bool             `dmr:"bit:0"`
	ProtectFlag bool             `dmr:"bit:1"`
	Opcode      CSBKOpcode       `dmr:"bits:2-7"`
	Reserved1   byte             `dmr:"bits:8-15"`
	Cdeftype    uint8            `dmr:"bits:16-19"`
	Reserved2   uint8            `dmr:"bits:20-21"`
	CdefParms   layer3.CdefParms `dmr:"bits:22-79,delegate"`
}

// DataType returns enums.DataTypeMBCContinuation.
func (s *BCAPContinuation) DataType() enums.DataType {
	return enums.DataTypeMBCContinuation
}

// VNAPContinuation represents Vote Now Absolute Parameters.
// Used with C_BCAST (opcode 0x28) Vote_Now when CH_VOTE=0xFFF.
// ETSI TS 102 361-4 — §7.2.19.3.1, Table 7.72
//
//dmr:crc crc_ccitt
//dmr:input_size 96
//dmr:subpdu
type VNAPContinuation struct {
	FEC         fec.Result       `dmr:"-"`
	LastBlock   bool             `dmr:"bit:0"`
	ProtectFlag bool             `dmr:"bit:1"`
	Opcode      CSBKOpcode       `dmr:"bits:2-7"`
	Reserved1   uint8            `dmr:"bits:8-11"`
	ColorCode   enums.ColorCode  `dmr:"bits:12-15"`
	Cdeftype    uint8            `dmr:"bits:16-19"`
	Reserved2   uint8            `dmr:"bits:20-21"`
	CdefParms   layer3.CdefParms `dmr:"bits:22-79,delegate"`
}

// DataType returns enums.DataTypeMBCContinuation.
func (s *VNAPContinuation) DataType() enums.DataType {
	return enums.DataTypeMBCContinuation
}
