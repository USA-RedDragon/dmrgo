package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2/crc"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	reedsolomon "github.com/USA-RedDragon/dmrgo/v2/fec/reedsolomon"
)

// MMDVMHostAccepts reports whether the 96-bit BPTC(196,96) payload of a
// burst of dataType, as Burst.BPTCPayload returns it, passes the check that
// MMDVMHost (g4klx/MMDVMHost 0acb758) applies before it uses a data burst
// from the network in CDMRSlot::writeNetwork. File and line references are
// to that commit.
//
//   - Voice LC Header and Terminator with LC: the RS(12,9) parity, with the
//     Data Type CRC Mask 0x969696 or 0x999999 removed, must equal the parity
//     of the 9 LC octets. No error is corrected, as in CRS129::check
//     (RS129.cpp:123-131) called by CDMRFullLC::decode (DMRFullLC.cpp:40-69).
//     A pdu.FullLinkControl passes this check when its FEC has no error and
//     no corrected error. MMDVMHost decodes the LC of a network Voice LC
//     Header and, when the check fails, replaces it with an LC built from
//     the DMRD header fields (DMRSlot.cpp:1158-1163). On RF it discards a
//     Voice LC Header that fails it (DMRSlot.cpp:241-244). It does not
//     decode a Terminator with LC on either path, but builds a new one from
//     the header LC (DMRSlot.cpp:377-379, 1332-1334).
//   - CSBK, MBC Header and Data Header: the CRC-CCITT with the mask 0xA5A5,
//     0xAAAA or 0xCCCC removed must check (CDMRCSBK::put, DMRCSBK.cpp:60-88;
//     CDMRDataHeader::put, DMRDataHeader.cpp:57-69). MMDVMHost discards the
//     burst when it fails (DMRSlot.cpp:1374-1378, 1690-1694, 1793-1797).
//     It also discards a BS_Dwn_Act CSBK or MBC Header (DMRSlot.cpp:1697,
//     1800), which this function does not check.
//   - MBC Continuation: CDMRCSBK::put checks the last 16 bits of every block
//     as a CRC-CCITT of its first 80 bits, without a mask (DMRCSBK.cpp:78-79,
//     86), and MMDVMHost discards a block that fails (DMRSlot.cpp:1898-1902).
//     ETSI TS 102 361-1 §7.4.0 instead puts one CRC, over all continuation
//     blocks, in the last block only; pdu.CheckMBCLastBlockCRC checks that
//     CRC. MMDVMHost thus discards the intermediate blocks of a standard MBC,
//     and its last block unless it is the only continuation block.
//   - PI Header, Rate 1/2, Rate 3/4 and Rate 1 Data: MMDVMHost checks no CRC
//     and forwards the burst (DMRSlot.cpp:1241-1327, 1999-2052). It ignores
//     the result of the Rate 3/4 trellis decode except for a log message
//     (DMRSlot.cpp:2017-2025). MMDVMHostAccepts returns true.
//   - Idle, Unified Single Block Data and reserved Data Types: writeNetwork
//     has no branch for them and discards the burst (DMRSlot.cpp:2053-2055),
//     so MMDVMHostAccepts returns false.
//
// MMDVMHost also discards some bursts by call state, for example data
// blocks outside a data call. MMDVMHostAccepts does not check call state.
func MMDVMHostAccepts(dataType enums.DataType, payload [12]byte) bool {
	switch dataType {
	case enums.DataTypeVoiceLCHeader:
		return rs129Matches(payload, 0x96)
	case enums.DataTypeTerminatorWithLC:
		return rs129Matches(payload, 0x99)
	case enums.DataTypeCSBK:
		return crcCCITTMatches(payload, 0xA5)
	case enums.DataTypeMBCHeader:
		return crcCCITTMatches(payload, 0xAA)
	case enums.DataTypeDataHeader:
		return crcCCITTMatches(payload, 0xCC)
	case enums.DataTypeMBCContinuation:
		return crcCCITTMatches(payload, 0x00)
	case enums.DataTypePIHeader,
		enums.DataTypeRate12,
		enums.DataTypeRate34,
		enums.DataTypeRate1:
		return true
	case enums.DataTypeIdle, enums.DataTypeUnifiedSingleBlock, enums.DataTypeReserved:
		return false
	default:
		return false
	}
}

func rs129Matches(payload [12]byte, mask byte) bool {
	codeword := reedsolomon.Encode([9]byte(payload[:9]))
	for i := 9; i < 12; i++ {
		if codeword[i] != payload[i]^mask {
			return false
		}
	}
	return true
}

func crcCCITTMatches(payload [12]byte, mask byte) bool {
	payload[10] ^= mask
	payload[11] ^= mask
	return crc.CheckCRCCCITT(payload[:])
}
