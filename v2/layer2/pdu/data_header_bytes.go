package pdu

import "github.com/USA-RedDragon/dmrgo/v2/enums"

// DataHeaderBytes is the 12-octet BPTC(196,96) payload of a data header
// burst (ETSI TS 102 361-1 §9.2), as layer2.RawBurst.BPTCPayload returns
// it: 80 header bits followed by the CRC-CCITT XORed with the Data Header
// mask 0xCCCC. Its methods read or write one field in place and leave
// every other bit as it is. The setters do not update the CRC: call Reseal
// after the last edit, or the header fails its CRC check.
//
// In the C_HEAD, U_HEAD, SP_HEAD, R_HEAD, DD_HEAD and UDT_HEAD formats
// (Tables 9.10, 9.15, 9.17A to 9.17D), bit 7 of octet 0 is the Group or
// Individual flag, octets 2 to 4 the destination LLID and octets 5 to 7
// the source LLID. The C_RHEAD format (Table 9.13) has the LLIDs at the
// same octets, but bit 7 of octet 0 is reserved. The P_HEAD format (Table
// 9.17) carries no LLIDs.
type DataHeaderBytes [12]byte

// Format returns the Format, the low 4 bits of octet 0.
func (h DataHeaderBytes) Format() Format {
	return Format(h[0] & 0x0F)
}

func (h DataHeaderBytes) hasGroupFlag() bool {
	switch h.Format() {
	case FormatUnifiedDataTransport, FormatUnconfirmed, FormatConfirmed,
		FormatShortDataDefined, FormatShortDataRawOrStatusPrecoded:
		return true
	case FormatResponsePacket, FormatProprietary:
		return false
	default:
		return false
	}
}

func (h DataHeaderBytes) hasLLIDs() bool {
	return h.hasGroupFlag() || h.Format() == FormatResponsePacket
}

// Addresses returns the destination LLID as target and the source LLID of
// the header. ok is false for a P_HEAD and for the reserved formats.
func (h DataHeaderBytes) Addresses() (target, source uint32, ok bool) {
	if !h.hasLLIDs() {
		return 0, 0, false
	}
	return uint24(h[2:5]), uint24(h[5:8]), true
}

// SetAddresses writes the destination and source LLIDs. It does not update
// the CRC; call Reseal. It returns false, and changes nothing, for a P_HEAD
// or a reserved format and when an LLID does not fit in 24 bits.
func (h *DataHeaderBytes) SetAddresses(target, source uint32) bool {
	if !h.hasLLIDs() || target>>24 != 0 || source>>24 != 0 {
		return false
	}
	putUint24(h[2:5], target)
	putUint24(h[5:8], source)
	return true
}

// Group returns the Group or Individual flag, bit 7 of octet 0: true for a
// group destination. ok is false for a C_RHEAD, whose bit 7 is reserved, a
// P_HEAD and the reserved formats.
func (h DataHeaderBytes) Group() (group, ok bool) {
	if !h.hasGroupFlag() {
		return false, false
	}
	return h[0]&0x80 != 0, true
}

// SetGroup writes the flag that Group returns. It does not update the CRC;
// call Reseal. It returns false, and changes nothing, for a header for
// which Group returns no flag.
func (h *DataHeaderBytes) SetGroup(group bool) bool {
	if !h.hasGroupFlag() {
		return false
	}
	if group {
		h[0] |= 0x80
	} else {
		h[0] &^= 0x80
	}
	return true
}

// CRCOK reports whether octets 10 and 11 hold the CRC of octets 0 to 9 with
// the Data Header mask, as BlockCRCOK does.
func (h DataHeaderBytes) CRCOK() bool {
	return BlockCRCOK(h, enums.DataTypeDataHeader)
}

// Reseal writes the CRC of octets 0 to 9 with the Data Header mask into
// octets 10 and 11, as Seal does.
func (h *DataHeaderBytes) Reseal() {
	_ = Seal((*[12]byte)(h), enums.DataTypeDataHeader)
}
