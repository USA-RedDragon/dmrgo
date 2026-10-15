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

// Addresses returns the destination and source LLIDs and the Group or
// Individual flag of the header. For a C_RHEAD group is false. ok is false
// for a P_HEAD and for the reserved formats.
func (h DataHeaderBytes) Addresses() (target, source uint32, group, ok bool) {
	if !h.hasLLIDs() {
		return 0, 0, false, false
	}
	target = uint32(h[2])<<16 | uint32(h[3])<<8 | uint32(h[4])
	source = uint32(h[5])<<16 | uint32(h[6])<<8 | uint32(h[7])
	return target, source, h.hasGroupFlag() && h[0]&0x80 != 0, true
}

// SetAddresses writes the destination and source LLIDs and the Group or
// Individual flag. It does not update the CRC; call Reseal. It returns
// false, and changes nothing, for a P_HEAD or a reserved format, for a
// C_RHEAD when group is true, and when an LLID does not fit in 24 bits. A
// C_RHEAD keeps its reserved bit 7 of octet 0.
func (h *DataHeaderBytes) SetAddresses(target, source uint32, group bool) bool {
	if !h.hasLLIDs() || target>>24 != 0 || source>>24 != 0 || group && !h.hasGroupFlag() {
		return false
	}
	switch {
	case !h.hasGroupFlag():
	case group:
		h[0] |= 0x80
	default:
		h[0] &^= 0x80
	}
	h[2], h[3], h[4] = byte(target>>16), byte(target>>8), byte(target) //nolint:gosec // 24-bit LLID split into octets
	h[5], h[6], h[7] = byte(source>>16), byte(source>>8), byte(source) //nolint:gosec // 24-bit LLID split into octets
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
