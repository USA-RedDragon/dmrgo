package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

// CSBKBytes is the 12-octet BPTC(196,96) payload of a CSBK burst, as
// layer2.RawBurst.BPTCPayload returns it: 80 CSBK bits and the CRC-CCITT
// XORed with the CSBK mask 0xA5A5 (ETSI TS 102 361-1 clause 9.1.8). Its
// methods read or write one field in place. The setters do not update the
// CRC: call Reseal after the last edit.
type CSBKBytes [12]byte

// Opcode returns the 6-bit CSBKO, the low 6 bits of octet 0.
func (c CSBKBytes) Opcode() CSBKOpcode {
	return CSBKOpcode(c[0] & 0x3F)
}

// FID returns the Feature set ID, octet 1.
func (c CSBKBytes) FID() enums.FeatureSetID {
	return enums.FeatureSetID(c[1])
}

func (c CSBKBytes) addressOffsets() (target, source int, ok bool) {
	if c.FID() != enums.FIDStandardized {
		return 0, 0, false
	}
	switch c.Opcode() {
	case CSBKNegativeAcknowledgement:
		return 7, 4, true
	case CSBKUnitToUnitVoiceServiceRequest, CSBKUnitToUnitVoiceServiceAnswerResponse,
		CSBKPreamble, CSBKPrivateVoiceGrant, CSBKTalkgroupVoiceGrant,
		CSBKBroadcastTalkgroupVoiceGrant, CSBKPrivateDataGrant, CSBKTalkgroupDataGrant,
		CSBKDuplexPrivateVoiceGrant, CSBKDuplexPrivateDataGrant,
		CSBKPrivateDataGrantMultiItem, CSBKClear, CSBKProtect, CSBKAhoy,
		CSBKRandomAccess, CSBKAckvitation, CSBKMaintenance, CSBKBSOutboundActivation:
		return 4, 7, true
	case CSBKChannelTiming, CSBKAloha, CSBKAckOutbound,
		CSBKAckInbound, CSBKAckOutboundPayload, CSBKAckInboundPayload,
		CSBKBroadcast, CSBKMove:
		return 0, 0, false
	default:
		return 0, 0, false
	}
}

// Addresses returns the 24-bit target and source addresses of the CSBKs for
// which CSBK.Addresses returns them, and ok false for every other CSBK.
// Octets 4 to 6 hold the target and octets 7 to 9 the source, except in
// NACK_Rsp, where they are the other way around (ETSI TS 102 361-2 Table 7.6).
func (c CSBKBytes) Addresses() (target, source uint32, ok bool) {
	t, s, ok := c.addressOffsets()
	if !ok {
		return 0, 0, false
	}
	return uint24(c[t:]), uint24(c[s:]), true
}

// SetAddresses writes target and source into the address octets of a CSBK
// for which Addresses returns ok. It does not update the CRC; call Reseal.
// It returns false, and changes nothing, for other CSBKs or when an address
// does not fit in 24 bits, as CSBK.SetAddresses does.
func (c *CSBKBytes) SetAddresses(target, source uint32) bool {
	t, s, ok := c.addressOffsets()
	if !ok || target>>24 != 0 || source>>24 != 0 {
		return false
	}
	putUint24(c[t:], target)
	putUint24(c[s:], source)
	return true
}

func (c CSBKBytes) groupBit() (octet int, mask byte, ok bool) {
	if c.FID() != enums.FIDStandardized {
		return 0, 0, false
	}
	switch c.Opcode() {
	case CSBKPreamble:
		return 2, 0x40, true
	case CSBKClear, CSBKProtect:
		return 3, 0x01, true
	case CSBKAhoy:
		return 3, 0x40, true
	case CSBKUnitToUnitVoiceServiceRequest, CSBKUnitToUnitVoiceServiceAnswerResponse,
		CSBKChannelTiming, CSBKNegativeAcknowledgement, CSBKBSOutboundActivation,
		CSBKAloha, CSBKAckvitation, CSBKRandomAccess, CSBKAckOutbound,
		CSBKAckInbound, CSBKAckOutboundPayload, CSBKAckInboundPayload,
		CSBKBroadcast, CSBKMaintenance, CSBKPrivateVoiceGrant,
		CSBKTalkgroupVoiceGrant, CSBKBroadcastTalkgroupVoiceGrant,
		CSBKPrivateDataGrant, CSBKTalkgroupDataGrant, CSBKDuplexPrivateVoiceGrant,
		CSBKDuplexPrivateDataGrant, CSBKPrivateDataGrantMultiItem, CSBKMove:
		return 0, 0, false
	default:
		return 0, 0, false
	}
}

// Group reports whether the target address is a talkgroup, from the Group
// bit of Pre_CSBK or the G/I bit of P_CLEAR, P_PROTECT, C_AHOY and P_AHOY
// (ETSI TS 102 361-2 Table 7.7 and ETSI TS 102 361-4 clause 7.1.1). ok is
// false for every other CSBK and for any FID other than the standard FID.
func (c CSBKBytes) Group() (group, ok bool) {
	o, m, ok := c.groupBit()
	if !ok {
		return false, false
	}
	return c[o]&m != 0, true
}

// SetGroup writes the flag that Group returns. It does not update the CRC;
// call Reseal. It returns false, and changes nothing, for a CSBK for which
// Group returns no flag.
func (c *CSBKBytes) SetGroup(group bool) bool {
	o, m, ok := c.groupBit()
	if !ok {
		return false
	}
	if group {
		c[o] |= m
	} else {
		c[o] &^= m
	}
	return true
}

// CRCOK reports whether octets 10 and 11 hold the CRC of octets 0 to 9 with
// the CSBK mask, as BlockCRCOK does.
func (c CSBKBytes) CRCOK() bool {
	return BlockCRCOK(c, enums.DataTypeCSBK)
}

// Reseal writes the CRC of octets 0 to 9 with the CSBK mask into octets 10
// and 11, as Seal does.
func (c *CSBKBytes) Reseal() {
	_ = Seal((*[12]byte)(c), enums.DataTypeCSBK)
}

// The *dmrgo.FieldRangeError values of the addresses of NewVoiceLCBytes and
// NewVoiceLC.
var (
	errLCBytesTargetRange         = addressRangeError("LCBytes", "target")
	errLCBytesSourceRange         = addressRangeError("LCBytes", "source")
	errFullLinkControlTargetRange = addressRangeError("FullLinkControl", "target")
	errFullLinkControlSourceRange = addressRangeError("FullLinkControl", "source")
)

// addressRangeError returns the *dmrgo.FieldRangeError of a 24-bit address
// field of type typ.
func addressRangeError(typ, field string) error {
	return &dmrgo.FieldRangeError{Type: typ, Field: field, Bits: 24}
}

// checkAddresses returns targetErr when target, or sourceErr when source,
// does not fit in 24 bits.
func checkAddresses(target, source uint32, targetErr, sourceErr error) error {
	switch {
	case target>>24 != 0:
		return targetErr
	case source>>24 != 0:
		return sourceErr
	}
	return nil
}

func uint24(b []byte) uint32 {
	return uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2])
}

func putUint24(b []byte, v uint32) {
	b[0], b[1], b[2] = byte(v>>16&0xFF), byte(v>>8&0xFF), byte(v&0xFF)
}
